// Package ops: operational CLI commands — doctor, backup, update.
package ops

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"net/url"

	"github.com/DanMotive/Todorio/internal/auth"
	"github.com/DanMotive/Todorio/internal/config"
	"github.com/DanMotive/Todorio/internal/db"
	"github.com/DanMotive/Todorio/internal/setup"
	"github.com/DanMotive/Todorio/internal/term"
)

const backupsDir = "/var/lib/todorio/backups"
const repo = "DanMotive/Todorio"

func ok(msg string)   { fmt.Println(" ", term.Green("[OK]"), msg) }
func bad(msg string)  { fmt.Println(" ", term.Red("[FAIL]"), msg) }
func warn(msg string) { fmt.Println(" ", term.Yellow("[WARN]"), msg) }

// certExpiry reads a PEM certificate file and returns its NotAfter (expiry) date.
func certExpiry(certFile string) (time.Time, error) {
	data, err := os.ReadFile(certFile)
	if err != nil {
		return time.Time{}, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return time.Time{}, fmt.Errorf("not a PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, err
	}
	return cert.NotAfter, nil
}

// Status — installation diagnostics (formerly `todorio doctor`).
func Status(cfg config.Config, version string) error {
	fmt.Println(term.Bold("todorio status"), "·", version)

	// config
	if _, err := os.Stat(config.Path()); err == nil {
		ok("config: " + config.Path())
	} else {
		bad("config not found: " + config.Path() + " — run `todorio setup`")
	}

	// DB and migrations
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if d, err := db.Connect(ctx, cfg.DatabaseURL); err == nil {
		var migrations, users int
		_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&migrations)
		_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users)
		ok(fmt.Sprintf("PostgreSQL reachable · migrations: %d · users: %d", migrations, users))
		d.Pool.Close()
	} else {
		bad("DB unreachable: " + err.Error())
	}

	// uploads storage
	if err := os.MkdirAll(cfg.UploadsDir, 0o750); err == nil {
		probe := filepath.Join(cfg.UploadsDir, ".doctor_probe")
		if err := os.WriteFile(probe, []byte("ok"), 0o600); err == nil {
			_ = os.Remove(probe)
			ok("storage is writable: " + cfg.UploadsDir)
		} else {
			bad("storage is not writable: " + cfg.UploadsDir)
		}
	} else {
		bad("could not create uploads directory: " + err.Error())
	}

	// SSL
	if cfg.HTTPS {
		if _, e1 := os.Stat(cfg.CertFile); e1 == nil {
			if _, e2 := os.Stat(cfg.KeyFile); e2 == nil {
				if expiry, eerr := certExpiry(cfg.CertFile); eerr == nil {
					days := int(time.Until(expiry).Hours() / 24)
					switch {
					case days < 0:
						bad(fmt.Sprintf("certificate expired on %s", expiry.Format("2006-01-02")))
					case days <= 30:
						warn(fmt.Sprintf("certificate expires in %d day(s) (%s) — consider renewing", days, expiry.Format("2006-01-02")))
					default:
						ok(fmt.Sprintf("certificate and key present, valid until %s (%d days)", expiry.Format("2006-01-02"), days))
					}
				} else {
					ok("certificate and key present")
				}
			} else {
				bad("key not found: " + cfg.KeyFile)
			}
		} else {
			bad("certificate not found: " + cfg.CertFile)
		}
	} else {
		warn("HTTPS is disabled — PWA install and browser push notifications will not work")
	}

	// backups
	if entries, err := os.ReadDir(backupsDir); err == nil && len(entries) > 0 {
		ok(fmt.Sprintf("backups: %d · %s", len(entries), backupsDir))
	} else {
		warn("no backups yet — `todorio backup create`")
	}

	// pg_dump for backups
	if _, err := exec.LookPath("pg_dump"); err == nil {
		ok("pg_dump found")
	} else {
		warn("pg_dump not found — install postgresql-client for backups")
	}

	// server URL, ready to copy
	scheme := "http"
	if cfg.HTTPS {
		scheme = "https"
	}
	host := setup.DetectPublicIP()
	if host == "" {
		host = "localhost"
	}
	fmt.Println()
	fmt.Println(" ", term.Cyan("Server:"), fmt.Sprintf("%s://%s:%d", scheme, host, cfg.Port))
	return nil
}

// Backup — pg_dump to gzip + an archive of uploads.
func Backup(cfg config.Config) error {
	if err := os.MkdirAll(backupsDir, 0o750); err != nil {
		return fmt.Errorf("backups directory: %w", err)
	}
	ts := time.Now().Format("20060102-150405")

	// 1. DB dump
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
		return fmt.Errorf("pg_dump did not start (is postgresql-client installed?): %w", err)
	}
	if _, err := io.Copy(gz, stdout); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("pg_dump exited with an error: %w", err)
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	ok("DB dump: " + dumpPath)

	// 2. archive of uploads
	if _, err := os.Stat(cfg.UploadsDir); err == nil {
		tarPath := filepath.Join(backupsDir, "uploads-"+ts+".tar.gz")
		tarCmd := exec.Command("tar", "-czf", tarPath,
			"-C", filepath.Dir(cfg.UploadsDir), filepath.Base(cfg.UploadsDir))
		tarCmd.Stderr = os.Stderr
		if err := tarCmd.Run(); err != nil {
			return fmt.Errorf("uploads archive: %w", err)
		}
		ok("attachments: " + tarPath)
	}
	fmt.Println(term.Green("Backup ready."), "Restore: gunzip -c <dump> | psql <database_url>")
	return nil
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// Update — update to the latest GitHub release, verifying sha256.
func Update(version string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/" + repo + "/releases/latest")
	if err != nil {
		return fmt.Errorf("GitHub unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub responded %s (no releases published yet?)", resp.Status)
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return err
	}
	if strings.TrimPrefix(rel.TagName, "v") == strings.TrimPrefix(version, "v") {
		fmt.Println(term.Green("Already up to date:"), version)
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
		return fmt.Errorf("release %s has no binary %s", rel.TagName, wantAsset)
	}
	fmt.Println(term.Cyan("Downloading"), rel.TagName, "...")

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

	// verify checksum if the release includes checksums.txt
	if sumURL != "" {
		sums, err := client.Get(sumURL)
		if err != nil {
			return err
		}
		defer sums.Body.Close()
		body, _ := io.ReadAll(sums.Body)
		if !strings.Contains(string(body), gotSum) {
			return fmt.Errorf("sha256 mismatch — update aborted")
		}
		ok("sha256 verified")
	} else {
		warn("checksums.txt not in the release — installing without verification")
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
		// cross-device rename — copy instead
		if err := copyFile(tmp.Name(), exe); err != nil {
			return err
		}
	}
	fmt.Println(term.Green("Updated to"), rel.TagName, "— restart the service (systemctl restart todorio)")
	return nil
}

// Uninstall removes Todorio from this machine.
//
// By default this stops the service and removes the binary, the systemd unit,
// and the config (/etc/todorio) — but keeps application data (/var/lib/todorio:
// uploads, backups) and the database, in case this was a mistake.
//   - --saveconfig also keeps the config directory (handy for reinstalling
//     with the same settings).
//   - --purge additionally removes application data and drops the database.
func Uninstall(cfg config.Config, purge bool, saveConfig bool, yes bool) error {
	fmt.Println(term.Bold("Uninstalling Todorio..."))

	if !yes {
		fmt.Print("This will stop the todorio service and remove the binary")
		if !saveConfig {
			fmt.Print(" and its config (/etc/todorio)")
		}
		if purge {
			fmt.Print(", and PERMANENTLY delete all application data and the database")
		}
		fmt.Print(". Continue? [y/N] ")
		var resp string
		fmt.Scanln(&resp)
		resp = strings.ToLower(strings.TrimSpace(resp))
		if resp != "y" && resp != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// stop and disable the systemd service, if present
	if _, err := exec.LookPath("systemctl"); err == nil {
		_ = exec.Command("systemctl", "disable", "--now", "todorio").Run()
	}
	if _, err := os.Stat("/etc/systemd/system/todorio.service"); err == nil {
		_ = os.Remove("/etc/systemd/system/todorio.service")
		_ = exec.Command("systemctl", "daemon-reload").Run()
		ok("removed the systemd service")
	}

	// binary
	removed := false
	if exe, err := os.Executable(); err == nil {
		if err := os.Remove(exe); err == nil {
			ok("removed binary: " + exe)
			removed = true
		}
	}
	if !removed {
		_ = os.Remove("/usr/local/bin/todorio")
	}

	// frontend + migrations installed alongside the binary
	_ = os.RemoveAll("/usr/share/todorio")

	// config: removed by default, kept with --saveconfig
	if saveConfig {
		warn("kept config: /etc/todorio (--saveconfig)")
	} else if err := os.RemoveAll("/etc/todorio"); err != nil {
		bad("could not remove config: " + err.Error())
	} else {
		ok("removed config: /etc/todorio")
	}

	if !purge {
		fmt.Println(term.Green("Todorio has been removed.") + " Application data and the database were kept" +
			" (data: /var/lib/todorio). Re-run with --purge to remove those too.")
		return nil
	}

	// purge: application data + database
	if err := os.RemoveAll("/var/lib/todorio"); err != nil {
		bad("could not remove /var/lib/todorio: " + err.Error())
	} else {
		ok("removed /var/lib/todorio (uploads, backups)")
	}

	if dbName, err := dbNameFromURL(cfg.DatabaseURL); err == nil && dbName != "" {
		dropCmd := exec.Command("dropdb", "--if-exists", dbName)
		dropCmd.Stderr = os.Stderr
		if err := dropCmd.Run(); err != nil {
			warn("could not drop database " + dbName + " automatically — drop it manually if needed: " + err.Error())
		} else {
			ok("dropped database: " + dbName)
		}
	} else {
		warn("could not determine the database name from the config — drop it manually if needed")
	}

	fmt.Println(term.Green("Todorio and all of its data have been removed."))
	return nil
}

// dbNameFromURL extracts the database name from a postgres connection URL.
func dbNameFromURL(dbURL string) (string, error) {
	u, err := url.Parse(dbURL)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(u.Path, "/"), nil
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

// ResetRoot resets the root admin's username and password: generates a fresh
// temporary password (shown once, never logged), optionally renames the
// account, forces a password change on next login, and logs the root admin
// out everywhere by clearing their sessions.
func ResetRoot(cfg config.Config, newUsername string, yes bool) error {
	if !yes {
		fmt.Print("This will reset the root admin's username and password, and log them out everywhere. Continue? [y/N] ")
		var resp string
		fmt.Scanln(&resp)
		resp = strings.ToLower(strings.TrimSpace(resp))
		if resp != "y" && resp != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("DB unreachable: %w", err)
	}
	defer d.Pool.Close()

	var id int64
	var username string
	if err := d.Pool.QueryRow(ctx, `SELECT id, username FROM users WHERE role='root' ORDER BY id LIMIT 1`).Scan(&id, &username); err != nil {
		return fmt.Errorf("no root admin found — register the first user or run `todorio setup`")
	}

	newUsername = strings.TrimSpace(newUsername)
	if newUsername == "" {
		newUsername = username
	} else if newUsername != username {
		var exists bool
		_ = d.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE username=$1 AND id<>$2)`, newUsername, id).Scan(&exists)
		if exists {
			return fmt.Errorf("username %q is already taken", newUsername)
		}
	}

	password, err := setup.GeneratePassword()
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	if _, err := d.Pool.Exec(ctx,
		`UPDATE users SET username=$2, password_hash=$3, must_change_password=true WHERE id=$1`,
		id, newUsername, hash); err != nil {
		return fmt.Errorf("database error: %w", err)
	}
	_, _ = d.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, id)

	fmt.Println(term.Green("Root admin reset."))
	fmt.Println("   Username:", newUsername)
	fmt.Println("   Temporary password:", password)
	fmt.Println("  ", term.Yellow("NOTE"), "The password is shown ONCE and is not written to logs.")
	fmt.Println("   The site will require changing it on next login. All previous root sessions were logged out.")
	return nil
}
