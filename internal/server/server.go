// Package server: HTTP server — API, frontend static assets, and SSE realtime.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	assets "github.com/DanMotive/Todorio"
	"github.com/DanMotive/Todorio/internal/api"
	"github.com/DanMotive/Todorio/internal/auth"
	"github.com/DanMotive/Todorio/internal/config"
	"github.com/DanMotive/Todorio/internal/db"
	"github.com/DanMotive/Todorio/internal/demo"
	"github.com/DanMotive/Todorio/internal/events"
	"github.com/DanMotive/Todorio/internal/telegram"
	"github.com/DanMotive/Todorio/internal/worker"
)

// shutdownGrace is how long in-flight work gets to finish after a shutdown signal.
//
// Note that /api/events subscribers are long-lived by design and will never finish on their own,
// so a restart with active browser tabs open always waits out this full window before those
// connections are forced closed. EventSource reconnects by itself, so the tabs recover. Ten
// seconds is a compromise: long enough for a slow database write or an upload to land, short
// enough that `todorio restart` still feels immediate.
const shutdownGrace = 10 * time.Second

func Run(cfg config.Config, version string) error {
	// Structured logging. slog.SetDefault also redirects the standard log package, so every
	// existing log.Printf across the codebase starts producing structured records with a
	// timestamp and level attached, without having to rewrite each call site.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// This context is cancelled when a shutdown signal arrives, which also stops the background
	// worker and the Telegram poller below — both already take a context and were previously
	// handed one that was never cancelled.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
	// Idles (checking every 20s) until root configures a bot token — see internal/telegram.
	go telegram.Run(ctx, database)

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
			// Shown on the About page. Defaults point at the real project; root can change or
			// blank them, and a blank value simply hides the row.
			"source_url": database.Setting(r.Context(), "branding.source_url", "https://github.com/DanMotive/Todorio"),
			"donate_url": database.Setting(r.Context(), "branding.donate_url", "https://boosty.to/danter1/about"),
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

	// requireSameOrigin sits outside the auth middleware so a rejected cross-site request is
	// turned away before it costs a database round trip.
	handler := securityHeaders(cfg.HTTPS)(requireSameOrigin(auth.Middleware(database)(mux)))

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port) // listen on all interfaces — allow access by bare IP without a domain
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
		// http.ListenAndServe leaves every timeout at zero, i.e. infinite. A connection that
		// opens and then dribbles out a header a second holds a goroutine and its buffers for as
		// long as it likes; a few thousand of them (Slowloris) exhaust the server without ever
		// sending a complete request.
		//
		// Only the header phase and idle keep-alives are bounded. ReadTimeout is left unset on
		// purpose because it would also cap the body, and an attachment upload over a weak mobile
		// connection can legitimately take minutes. WriteTimeout is left unset because /api/events
		// is a long-lived SSE stream that any write deadline would cut off mid-flight.
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Printf("Todorio %s running at %s (https=%v)", version, addr, cfg.HTTPS)

	// Serving moves to its own goroutine so this one can wait for a signal.
	//
	// SIGTERM arrives on every `todorio restart`, every systemd stop and every deploy. Until
	// now the process died exactly where it stood: a handler midway through a sequence of
	// statements lost the rest of them, an upload being written to disk left a truncated file
	// with a database row already pointing at it, and every open SSE stream was cut without
	// notice. Shutdown stops accepting new connections and gives the work already in progress a
	// chance to finish.
	serveErr := make(chan error, 1)
	go func() {
		if cfg.HTTPS && cfg.CertFile != "" && cfg.KeyFile != "" {
			serveErr <- srv.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
			return
		}
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		// The listener failed on its own (port already in use, bad certificate).
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	// Restore the default signal behaviour: a second Ctrl-C or SIGTERM from an impatient
	// operator now terminates immediately instead of being swallowed by this handler.
	stop()
	log.Printf("shutdown: waiting up to %s for in-flight requests", shutdownGrace)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		// Expected whenever an SSE stream is still open, since those never go idle.
		log.Printf("shutdown: grace period expired (%v), closing remaining connections", err)
		_ = srv.Close()
	}
	log.Printf("shutdown complete")
	return nil
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
			// The headers above stop sniffing and framing but say nothing about what the page may
			// load or execute, so any HTML injection that slipped through (task titles, comments,
			// the Markdown renderer, a custom logo) could pull in an external script and read the
			// session. The frontend is a single bundled module with no inline script and no
			// third-party origins, so it fits within 'self' as-is; 'unsafe-inline' is granted for
			// styles only, which the bundler needs.
			h.Set("Content-Security-Policy", strings.Join([]string{
				"default-src 'self'",
				"script-src 'self'",
				"style-src 'self' 'unsafe-inline'",
				"img-src 'self' data: blob:",
				"font-src 'self' data:",
				"connect-src 'self'",
				"frame-ancestors 'none'",
				"base-uri 'none'",
				"form-action 'self'",
				"object-src 'none'",
			}, "; "))
			if https {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireSameOrigin blocks state-changing requests that did not originate from this site.
//
// The session cookie is SameSite=Lax, which the browser still attaches to a top-level navigation
// from anywhere — deliberately, so that a task link in a Telegram notification opens signed in.
// Lax does not cover a cross-site form submission, and the API sends no CSRF token of its own, so
// until now another site could POST to an endpoint on the user's behalf. Checking the origin
// server-side closes that without a token and without touching the frontend: browsers attach
// Sec-Fetch-Site and Origin to these requests automatically.
//
// Safe methods are untouched, so ordinary links, the SSE stream and the public share pages behave
// exactly as before. Note that a non-browser client (curl, a script) that sends none of these
// headers will be refused on writes; that is the intended trade, and such a caller only needs to
// pass -H "Origin: <site>".
func requireSameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if sameOrigin(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "cross-site request blocked"})
	})
}

func sameOrigin(r *http.Request) bool {
	// Sec-Fetch-Site is the most reliable signal where it exists: the browser fills it in and a
	// page cannot forge it. "none" means the user typed the URL or used a bookmark.
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin" || site == "none"
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		return hostMatches(origin, r.Host)
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		return hostMatches(ref, r.Host)
	}
	return false
}

func hostMatches(rawURL, host string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}
