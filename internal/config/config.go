// Package config — единый источник настроек: читается и root-панелью, и CLI.
// Файл: /etc/todorio/config.json (или $TODORIO_CONFIG).
// Динамические политики/лимиты/брендинг хранятся в БД (таблица system_settings),
// а CLI-команды `todorio server ... set` пишут туда же — без расхождений с вебом.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Port           int    `json:"port"`
	HTTPS          bool   `json:"https"`
	CertFile       string `json:"cert_file,omitempty"`
	KeyFile        string `json:"key_file,omitempty"`
	DatabaseURL    string `json:"database_url"`
	UploadsDir     string `json:"uploads_dir"`
	ProcessManager string `json:"process_manager"` // systemd | docker | pm2
	DefaultLocale  string `json:"default_locale"`
	DetectBrowser  bool   `json:"detect_browser_locale"`
	// Дефолты темы сервера (root); пользователь может переопределить в профиле.
	DefaultColor   string `json:"default_color"`  // red | blue | green | yellow | gray
	DefaultScheme  string `json:"default_scheme"` // light | dark
	DefaultVisual  string `json:"default_visual"` // rich | lite
}

func Defaults() Config {
	return Config{
		Port:           8080,
		DatabaseURL:    "postgres://todorio:todorio@localhost:5432/todorio",
		UploadsDir:     "/var/lib/todorio/uploads",
		ProcessManager: "systemd",
		DefaultLocale:  "en-US",
		DetectBrowser:  true,
		DefaultColor:   "blue",
		DefaultScheme:  "dark",
		DefaultVisual:  "rich",
	}
}

func Path() string {
	if p := os.Getenv("TODORIO_CONFIG"); p != "" {
		return p
	}
	return "/etc/todorio/config.json"
}

func Load() (Config, error) {
	cfg := Defaults()
	b, err := os.ReadFile(Path())
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("повреждён %s: %w", Path(), err)
	}
	return cfg, nil
}

func Save(cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(Path()), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(Path(), b, 0o600)
}

// RunCLI обрабатывает `todorio server <config|policy|limits|branding|locales> ...`.
// Скелет: печатает намерение; в полной версии пишет в БД system_settings.
func RunCLI(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("укажите раздел: config | policy | limits | branding | locales")
	}
	section := args[0]
	rest := args[1:]
	switch section {
	case "config", "policy", "limits", "branding":
		if len(rest) == 3 && rest[0] == "set" {
			fmt.Printf("OK: %s.%s = %q (TODO: запись в system_settings)\n", section, rest[1], rest[2])
			return nil
		}
		return fmt.Errorf("формат: todorio server %s set <ключ> <значение>", section)
	case "locales":
		if len(rest) == 2 && (rest[0] == "enable" || rest[0] == "disable") {
			fmt.Printf("OK: локаль %s → %s (TODO: запись в system_settings)\n", rest[1], rest[0])
			return nil
		}
		return fmt.Errorf("формат: todorio server locales enable|disable <locale>")
	}
	return fmt.Errorf("неизвестный раздел: %s", section)
}
