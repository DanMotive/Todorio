// Package server: HTTP server — API, frontend static assets, and SSE realtime.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/DanMotive/Todorio/internal/api"
	"github.com/DanMotive/Todorio/internal/auth"
	"github.com/DanMotive/Todorio/internal/config"
	"github.com/DanMotive/Todorio/internal/db"
	"github.com/DanMotive/Todorio/internal/demo"
	"github.com/DanMotive/Todorio/internal/events"
	"github.com/DanMotive/Todorio/internal/worker"
)

func Run(cfg config.Config, version string) error {
	ctx := context.Background()

	// --- DB and migrations ---
	database, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	// demo space with quests — created once, after migrations (see below, after Migrate)
	if err := database.Migrate(ctx, migrationsDir()); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	// --- event bus and background jobs ---
	bus := events.New()
	if err := demo.EnsureDemo(ctx, database); err != nil {
		log.Println("demo space:", err)
	}
	go worker.Run(ctx, database, bus)

	// --- routes ---
	mux := http.NewServeMux()
	a := &api.API{DB: database, Bus: bus, Cfg: cfg, Version: version}
	a.Routes(mux)

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "version": version})
	})

	// Public settings for the frontend before login: branding, locales, default theme.
	mux.HandleFunc("GET /api/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"site_name":      database.Setting(r.Context(), "branding.site_name", "Todorio"),
			"browser_title":  database.Setting(r.Context(), "branding.browser_title", "Todorio"),
			"developer_name": database.Setting(r.Context(), "branding.developer_name", "Vlad"),
			"default_locale": cfg.DefaultLocale,
			"detect_browser_locale": cfg.DetectBrowser,
			"registration_mode": database.Setting(r.Context(), "policy.registration.mode", "open_approval"),
			"theme": map[string]string{
				"color":  database.Setting(r.Context(), "branding.default_color", cfg.DefaultColor),
				"scheme": database.Setting(r.Context(), "branding.default_scheme", cfg.DefaultScheme),
				"visual": database.Setting(r.Context(), "branding.default_visual", cfg.DefaultVisual),
			},
		})
	})

	// Frontend static assets + SPA fallback to index.html.
	mux.Handle("/", spaHandler(webDistDir()))

	handler := securityHeaders(cfg.HTTPS)(auth.Middleware(database)(mux))

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port) // listen on all interfaces — allow access by bare IP without a domain
	log.Printf("Todorio %s running at %s (https=%v)", version, addr, cfg.HTTPS)
	if cfg.HTTPS && cfg.CertFile != "" && cfg.KeyFile != "" {
		return http.ListenAndServeTLS(addr, cfg.CertFile, cfg.KeyFile, handler)
	}
	return http.ListenAndServe(addr, handler)
}

// webDistDir — the built frontend: next to the binary (dev) or in /usr/share/todorio (prod).
func webDistDir() string {
	if _, err := os.Stat("web/dist"); err == nil {
		return "web/dist"
	}
	return "/usr/share/todorio/web/dist"
}

// spaHandler serves files from dist, and for client-side routes (/space/5 etc.) falls back to index.html.
func spaHandler(dist string) http.Handler {
	fs := http.FileServer(http.Dir(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		path := filepath.Join(dist, filepath.Clean(r.URL.Path))
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dist, "index.html"))
	})
}

func migrationsDir() string {
	// Next to the binary in prod (/usr/share/todorio/migrations), ./migrations in the repo.
	if _, err := os.Stat("/usr/share/todorio/migrations"); err == nil {
		return "/usr/share/todorio/migrations"
	}
	return "migrations"
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

// securityHeaders adds a conservative set of hardening headers to every response:
// no MIME sniffing, no framing by other sites (clickjacking), a strict referrer
// policy, and (when serving over HTTPS) HSTS so browsers stick to HTTPS afterwards.
func securityHeaders(https bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			if https {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}
