// Package server: HTTP server — API, frontend static assets, and SSE realtime.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	assets "github.com/DanMotive/Todorio"
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

	// --- DB and migrations (embedded in the binary — see assets.go) ---
	database, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	migrationsFS, err := fs.Sub(assets.Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("embedded migrations: %w", err)
	}
	if err := database.Migrate(ctx, migrationsFS); err != nil {
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
		allLocales := []string{
			"en-US", "ru-RU", "uk-UA", "be-BY", "kk-KZ",
			"es-ES", "pt-BR", "tr-TR",
			"zh-CN", "hi-IN", "bn-BD", "ja-JP", "ko-KR",
		}
		locales := allLocales
		if raw := database.Setting(r.Context(), "locales.enabled", "[]"); raw != "[]" {
			var enabled []string
			if json.Unmarshal([]byte(raw), &enabled) == nil && len(enabled) > 0 {
				locales = enabled
			}
		}
		writeJSON(w, map[string]any{
			"site_name":         database.Setting(r.Context(), "branding.site_name", "Todorio"),
			"browser_title":     database.Setting(r.Context(), "branding.browser_title", "Todorio"),
			"developer_name":    database.Setting(r.Context(), "branding.developer_name", "DanMotive"),
			"developer_url":     database.Setting(r.Context(), "branding.developer_url", ""),
			"footer_text":       database.Setting(r.Context(), "branding.footer_text", ""),
			"show_product_name": database.Setting(r.Context(), "branding.show_product_name", "true") != "false",
			"about_text":        database.Setting(r.Context(), "branding.about_text", ""),
			// Empty means "no custom logo" — the frontend falls back to the bundled SVG.
			"logo_path":             database.Setting(r.Context(), "branding.logo_path", ""),
			"version":               version,
			"default_locale":        cfg.DefaultLocale,
			"detect_browser_locale": cfg.DetectBrowser,
			"registration_mode":     database.Setting(r.Context(), "policy.registration.mode", "open_approval"),
			"locales_enabled":       locales,
			"theme": map[string]string{
				"color":  database.Setting(r.Context(), "branding.default_color", cfg.DefaultColor),
				"visual": database.Setting(r.Context(), "branding.default_visual", cfg.DefaultVisual),
			},
		})
	})

	// Frontend static assets + SPA fallback to index.html.
	dist, err := webFS()
	if err != nil {
		return err
	}
	mux.Handle("/", spaHandler(dist))

	handler := securityHeaders(cfg.HTTPS)(auth.Middleware(database)(mux))

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port) // listen on all interfaces — allow access by bare IP without a domain
	log.Printf("Todorio %s running at %s (https=%v)", version, addr, cfg.HTTPS)
	if cfg.HTTPS && cfg.CertFile != "" && cfg.KeyFile != "" {
		return http.ListenAndServeTLS(addr, cfg.CertFile, cfg.KeyFile, handler)
	}
	return http.ListenAndServe(addr, handler)
}

// webFS resolves the frontend to serve. Preference order:
//  1. The embedded build (assets.Web), if it's real — i.e. `npm run build` ran
//     before `go build`, as the release pipeline does, so the binary is fully
//     self-contained and needs no files alongside it.
//  2. ./web/dist on disk (running from a source checkout without embedding).
//  3. /usr/share/todorio/web/dist (older installs that copied files onto the
//     server instead of using a self-contained binary).
//
// Returns an error only if none of the three actually has a built frontend —
// a clear startup failure beats silently serving 404s for every page.
func webFS() (fs.FS, error) {
	embedded, err := fs.Sub(assets.Web, "web/dist")
	if err == nil {
		if _, err := fs.Stat(embedded, "index.html"); err == nil {
			return embedded, nil // real embedded frontend
		}
	}
	if _, err := os.Stat("web/dist/index.html"); err == nil {
		return os.DirFS("web/dist"), nil
	}
	if _, err := os.Stat("/usr/share/todorio/web/dist/index.html"); err == nil {
		return os.DirFS("/usr/share/todorio/web/dist"), nil
	}
	return nil, fmt.Errorf("no built frontend found (embedded in the binary, ./web/dist, or /usr/share/todorio/web/dist) — run `npm run build` in web/ before `go build`, or use a released binary")
}

// spaHandler serves files from dist, and for client-side routes (/space/5 etc.) falls back to index.html.
func spaHandler(dist fs.FS) http.Handler {
	fileServer := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		clean := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if st, err := fs.Stat(dist, clean); err == nil && !st.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFileFS(w, r, dist, "index.html")
	})
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
