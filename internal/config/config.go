// Package config is the single source of truth for settings: read by both the root panel and the CLI.
// File: /etc/todorio/config.json (or $TODORIO_CONFIG).
// Dynamic policies/limits/branding are stored in the DB (system_settings table),
// and the `todorio server ... set` CLI commands write to the same place — no drift from the web UI.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/DanMotive/Todorio/internal/db"
)

type Config struct {
	Port          int    `json:"port"`
	HTTPS         bool   `json:"https"`
	CertFile      string `json:"cert_file,omitempty"`
	KeyFile       string `json:"key_file,omitempty"`
	DatabaseURL   string `json:"database_url"`
	UploadsDir    string `json:"uploads_dir"`
	DefaultLocale string `json:"default_locale"`
	DetectBrowser bool   `json:"detect_browser_locale"`
	// Server-wide theme defaults (root); the user can override them in their profile.
	DefaultColor  string `json:"default_color"`  // red | blue | green | yellow | gray
	DefaultVisual string `json:"default_visual"` // rich | lite
}

func Defaults() Config {
	return Config{
		Port:          8080,
		DatabaseURL:   "postgres://todorio:todorio@localhost:5432/todorio",
		UploadsDir:    "/var/lib/todorio/uploads",
		DefaultLocale: "en-US",
		DetectBrowser: true,
		DefaultColor:  "blue",
		DefaultVisual: "rich",
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
// Writes to system_settings in the DB — the same table the root web panel reads and writes,
// so the CLI and the web UI are always a single source of truth (per spec section 10).
func RunCLI(args []string, cfg Config) error {
	if len(args) < 1 {
		return fmt.Errorf("specify a section: config | policy | limits | branding | locales")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	database, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to the database: %w", err)
	}
	defer database.Pool.Close()

	section := args[0]
	rest := args[1:]
	switch section {
	case "config", "policy", "limits", "branding":
		if len(rest) == 3 && rest[0] == "set" {
			key := section + "." + rest[1]
			b, err := json.Marshal(rest[2])
			if err != nil {
				return err
			}
			if err := database.SetSetting(ctx, key, string(b)); err != nil {
				return fmt.Errorf("writing %s: %w", key, err)
			}
			fmt.Printf("OK: %s = %q\n", key, rest[2])
			return nil
		}
		return fmt.Errorf("usage: todorio server %s set <key> <value>", section)
	case "locales":
		if len(rest) == 2 && (rest[0] == "enable" || rest[0] == "disable") {
			locale := rest[1]
			enabled := readLocaleList(ctx, database)
			enabled = toggleLocale(enabled, locale, rest[0] == "enable")
			b, _ := json.Marshal(enabled)
			if err := database.SetSetting(ctx, "locales.enabled", string(b)); err != nil {
				return fmt.Errorf("writing locales.enabled: %w", err)
			}
			fmt.Printf("OK: locale %s → %s\n", locale, rest[0])
			return nil
		}
		return fmt.Errorf("usage: todorio server locales enable|disable <locale>")
	}
	return fmt.Errorf("unknown section: %s", section)
}

// readLocaleList reads the locales.enabled setting (a JSON array of locale codes).
// An empty/missing setting means "all supported locales are enabled" (the default,
// out-of-the-box behavior) — this list only tracks explicit admin overrides.
func readLocaleList(ctx context.Context, d *db.DB) []string {
	raw := d.Setting(ctx, "locales.enabled", "[]")
	var list []string
	if json.Unmarshal([]byte(raw), &list) != nil {
		return []string{}
	}
	return list
}

func toggleLocale(list []string, locale string, add bool) []string {
	out := make([]string, 0, len(list)+1)
	found := false
	for _, l := range list {
		if l == locale {
			found = true
			if !add {
				continue // disable: drop it from the list
			}
		}
		out = append(out, l)
	}
	if add && !found {
		out = append(out, locale)
	}
	return out
}
