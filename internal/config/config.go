// Package config is the single source of truth for settings: read by both the root panel and the CLI.
// File: /etc/todorio/config.json (or $TODORIO_CONFIG).
// Dynamic policies/limits/branding are stored in the DB (system_settings table),
// and the `todorio server ... set` CLI commands write to the same place — no drift from the web UI.
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
	DefaultLocale  string `json:"default_locale"`
	DetectBrowser  bool   `json:"detect_browser_locale"`
	// Server-wide theme defaults (root); the user can override them in their profile.
	DefaultColor   string `json:"default_color"`  // red | blue | green | yellow | gray
	DefaultScheme  string `json:"default_scheme"` // light | dark
	DefaultVisual  string `json:"default_visual"` // rich | lite
}

func Defaults() Config {
	return Config{
		Port:           8080,
		DatabaseURL:    "postgres://todorio:todorio@localhost:5432/todorio",
		UploadsDir:     "/var/lib/todorio/uploads",
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
		return cfg, fmt.Errorf("corrupted %s: %w", Path(), err)
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

// RunCLI handles `todorio server <config|policy|limits|branding|locales> ...`.
// Skeleton: prints intent for now; the full version writes to system_settings in the DB.
func RunCLI(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("specify a section: config | policy | limits | branding | locales")
	}
	section := args[0]
	rest := args[1:]
	switch section {
	case "config", "policy", "limits", "branding":
		if len(rest) == 3 && rest[0] == "set" {
			fmt.Printf("OK: %s.%s = %q (TODO: write to system_settings)\n", section, rest[1], rest[2])
			return nil
		}
		return fmt.Errorf("usage: todorio server %s set <key> <value>", section)
	case "locales":
		if len(rest) == 2 && (rest[0] == "enable" || rest[0] == "disable") {
			fmt.Printf("OK: locale %s → %s (TODO: write to system_settings)\n", rest[1], rest[0])
			return nil
		}
		return fmt.Errorf("usage: todorio server locales enable|disable <locale>")
	}
	return fmt.Errorf("unknown section: %s", section)
}
