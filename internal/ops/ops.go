// Package ops — эксплуатационные команды CLI: doctor, backup, update.
package ops

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/DanMotive/Todorio/internal/config"
	"github.com/DanMotive/Todorio/internal/db"
)

const backupsDir = "/var/lib/todorio/backups"
const repo = "DanMotive/Todorio"

func ok(msg string)   { fmt.Println("  ✔", msg) }
func bad(msg string)  { fmt.Println("  ✖", msg) }
func warn(msg string) { fmt.Println("  ⚠", msg) }

// Doctor — диагностика установки.
func Doctor(cfg config.Config, version string) error {
	fmt.Println("🩺 todorio doctor ·", version)

	// конфиг
	if _, err := os.Stat(config.Path()); err == nil {
		ok("конфиг: " + config.Path())
	} else {
		bad("конфиг не найден: " + config.Path() + " — выполните `todorio setup`")
	}

	// БД и миграции
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if d, err := db.Connect(ctx, cfg.DatabaseURL); err == nil {
		var migrations, users int
		_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&migrations)
		_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users)
		ok(fmt.Sprintf("PostgreSQL доступен · миграций: %d · пользователей: %d", migrations, users))
		d.Pool.Close()
	} else {
		bad("БД недоступна: " + err.Error())
	}

	// хранилище загрузок
	if err := os.MkdirAll(cfg.UploadsDir, 0o750); err == nil {
		probe := filepath.Join(cfg.UploadsDir, ".doctor_probe")
		if err := os.WriteFile(probe, []byte("ok"), 0o600); err == nil {
			_ = os.Remove(probe)
			ok("хранилище записываемо: " + cfg.UploadsDir)
		} else {
			bad("хранилище недоступно для записи: " + cfg.UploadsDir)
		}
	} else {
		bad("не создаётся каталог загрузок: " + err.Error())
	}

	// SSL
	if cfg.HTTPS {
		if _, e1 := os.Stat(cfg.CertFile); e1 == nil {
			if _, e2 := os.Stat(cfg.KeyFile); e2 == nil {
				ok("сертификат и ключ на месте")
			} else {
				bad("не найден ключ: " + cfg.KeyFile)
			}
		} else {
			bad("не найден сертификат: " + cfg.CertFile)
		}
	} else {
		warn("HTTPS выключен — PWA и системные уведомления браузера работать не будут")
	}

	// бэкапы
	if entries, err := os.ReadDir(backupsDir); err == nil && len(entries) > 0 {
		ok(fmt.Sprintf("бэкапов: %d · %s", len(entries), backupsDir))
	} else {
		warn("бэкапов пока нет — `todorio backup create`")
	}

	// pg_dump для бэкапов
	if _, err := exec.LookPath("pg_dump"); err == nil {
		ok("pg_dump найден")
	} else {
		warn("pg_dump не найден — установите postgresql-client для бэкапов")
	}
	return nil
}

// Backup — pg_dump в gzip + архив загрузок.
func Backup(cfg config.Config) error {
	if err := os.MkdirAll(backupsDir, 0o750); err != nil {
		return fmt.Errorf("каталог бэкапов: %w", err)
	}
	ts := time.Now().Format("20060102-150405")

	// 1. дамп БД
	dumpPath := filepath.Join(backupsDir, "todorio-"+ts+".sql.gz")
	cmd := exec.Command("pg_dump", "--dbname="+cfg.DatabaseURL)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	f, err := os.Create(dumpPath)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pg_dump не запустился (установлен postgresql-client?): %w", err)
	}
	if _, err := io.Copy(gz, stdout); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("pg_dump завершился с ошибкой: %w", err)
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Println("✔ дамп БД:", dumpPath)

	// 2. архив загрузок
	if _, err := os.Stat(cfg.UploadsDir); err == nil {
		tarPath := filepath.Join(backupsDir, "uploads-"+ts+".tar.gz")
		tarCmd := exec.Command("tar", "-czf", tarPath,
			"-C", filepath.Dir(cfg.UploadsDir), filepath.Base(cfg.UploadsDir))
		tarCmd.Stderr = os.Stderr
		if err := tarCmd.Run(); err != nil {
			return fmt.Errorf("архив загрузок: %w", err)
		}
		fmt.Println("✔ вложения:", tarPath)
	}
	fmt.Println("✅ Бэкап готов. Восстановление: gunzip -c <дамп> | psql <database_url>")
	return nil
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// Update — обновление до последнего релиза GitHub с проверкой sha256.
func Update(version string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/" + repo + "/releases/latest")
	if err != nil {
		return fmt.Errorf("GitHub недоступен: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub ответил %s (релизы ещё не опубликованы?)", resp.Status)
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return err
	}
	if strings.TrimPrefix(rel.TagName, "v") == strings.TrimPrefix(version, "v") {
		fmt.Println("✅ Уже последняя версия:", version)
		return nil
	}

	wantAsset := fmt.Sprintf("todorio_%s_%s", runtime.GOOS, runtime.GOARCH)
	var binURL, sumURL string
	for _, a := range rel.Assets {
		switch a.Name {
		case wantAsset:
			binURL = a.BrowserDownloadURL
		case "checksums.txt":
			sumURL = a.BrowserDownloadURL
		}
	}
	if binURL == "" {
		return fmt.Errorf("в релизе %s нет бинарника %s", rel.TagName, wantAsset)
	}
	fmt.Println("⬇ Скачиваю", rel.TagName, "…")

	tmp, err := os.CreateTemp("", "todorio-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	bin, err := client.Get(binURL)
	if err != nil {
		return err
	}
	defer bin.Body.Close()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), bin.Body); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	gotSum := hex.EncodeToString(h.Sum(nil))

	// проверка суммы, если в релизе есть checksums.txt
	if sumURL != "" {
		sums, err := client.Get(sumURL)
		if err != nil {
			return err
		}
		defer sums.Body.Close()
		body, _ := io.ReadAll(sums.Body)
		if !strings.Contains(string(body), gotSum) {
			return fmt.Errorf("sha256 не совпадает — обновление отменено")
		}
		fmt.Println("✔ sha256 подтверждён")
	} else {
		fmt.Println("⚠ checksums.txt нет в релизе — устанавливаю без проверки")
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	_ = os.Rename(exe, exe+".old")
	if err := os.Rename(tmp.Name(), exe); err != nil {
		// кросс-девайс rename — копируем
		if err := copyFile(tmp.Name(), exe); err != nil {
			return err
		}
	}
	fmt.Println("✅ Обновлено до", rel.TagName, "— перезапустите сервис (systemctl restart todorio)")
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
