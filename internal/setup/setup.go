// Package setup — интерактивная первичная настройка `todorio setup`.
package setup

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"

	"github.com/DanMotive/Todorio/internal/config"
)

const passwordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789!@#$%^&*-_=+"

// GeneratePassword возвращает криптостойкий временный пароль из 16 символов.
func GeneratePassword() (string, error) {
	var b strings.Builder
	for i := 0; i < 16; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordAlphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(passwordAlphabet[n.Int64()])
	}
	return b.String(), nil
}

func ask(r *bufio.Reader, prompt, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", prompt, def)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func askYN(r *bufio.Reader, prompt string, def bool) bool {
	d := "y/N"
	if def {
		d = "Y/n"
	}
	ans := strings.ToLower(ask(r, prompt+" ("+d+")", ""))
	if ans == "" {
		return def
	}
	return ans == "y" || ans == "yes" || ans == "д" || ans == "да"
}

// Run задаёт вопросы, сохраняет конфиг и печатает временный пароль root-админа.
// TODO (полная версия): создание БД и root-пользователя (argon2id, must_change_password=true),
// генерация self-signed сертификата, установка unit-файла/compose/pm2,
// создание обучающего демо-пространства с квестами.
func Run() error {
	r := bufio.NewReader(os.Stdin)
	cfg := config.Defaults()

	fmt.Println("⚡ Todorio — первичная настройка")
	fmt.Println(strings.Repeat("─", 40))

	root := ask(r, "Логин root-администратора", "root")

	pm := ask(r, "Менеджер процессов (systemd/docker/pm2)", "systemd")
	switch pm {
	case "systemd", "docker", "pm2":
		cfg.ProcessManager = pm
	default:
		return fmt.Errorf("неизвестный менеджер процессов: %s", pm)
	}

	portStr := ask(r, "Порт сайта", "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("некорректный порт: %s", portStr)
	}
	cfg.Port = port

	cfg.HTTPS = askYN(r, "Включить HTTPS с self-signed сертификатом?", false)
	if cfg.HTTPS {
		hostsStr := ask(r, "IP/домены для сертификата (через запятую)", defaultCertHosts())
		hosts := []string{}
		for _, h := range strings.Split(hostsStr, ",") {
			if h = strings.TrimSpace(h); h != "" {
				hosts = append(hosts, h)
			}
		}
		certFile, keyFile, cerr := GenerateSelfSigned("/etc/todorio/ssl", hosts)
		if cerr != nil {
			fmt.Println("⚠ Не удалось сгенерировать сертификат:", cerr)
			fmt.Println("  HTTPS отключён. Укажите cert_file/key_file вручную и включите:")
			fmt.Println("  todorio server config set https true")
			cfg.HTTPS = false
		} else {
			cfg.CertFile, cfg.KeyFile = certFile, keyFile
			fmt.Println("🔐 Сертификат:", certFile, "(10 лет, браузер покажет предупреждение — это нормально)")
		}
	}
	demo := askYN(r, "Создать обучающее демо-пространство с заданиями-квестами?", true)

	password, err := GeneratePassword()
	if err != nil {
		return err
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("не удалось сохранить конфиг: %w", err)
	}

	fmt.Println()
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println("✅ Настройка завершена. Конфиг:", config.Path())
	fmt.Printf("   Root-админ: %s\n", root)
	fmt.Printf("   Временный пароль: %s\n", password)
	fmt.Println("   ⚠ Пароль показывается ОДИН раз и не пишется в логи.")
	fmt.Println("   При первом входе сайт потребует сменить его.")
	if demo {
		fmt.Println("   🎓 Демо-пространство с квестами будет создано при первом запуске.")
	}
	fmt.Printf("   Запуск: sudo systemctl start todorio (или `todorio serve`)\n")
	return nil
}
