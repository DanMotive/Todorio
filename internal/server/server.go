// Package server — HTTP-сервер: API, статика фронтенда и SSE-реалтайм.
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

	// --- БД и миграции ---
	database, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	// демо-пространство с квестами — создаётся один раз после миграций (см. ниже после Migrate)
	if err := database.Migrate(ctx, migrationsDir()); err != nil {
		return fmt.Errorf("миграции: %w", err)
	}

	// --- шина событий и фоновые задачи ---
	bus := events.New()
	if err := demo.EnsureDemo(ctx, database); err != nil {
		log.Println("демо-пространство:", err)
	}
	go worker.Run(ctx, database, bus)

	// --- маршруты ---
	mux := http.NewServeMux()
	a := &api.API{DB: database, Bus: bus, Cfg: cfg, Version: version}
	a.Routes(mux)

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "version": version})
	})

	// Публичные настройки для фронта до логина: брендинг, локали, тема по умолчанию.
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

	// Статика фронтенда + SPA-fallback на index.html.
	mux.Handle("/", spaHandler("web/dist"))

	handler := auth.Middleware(database)(mux)

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port) // слушаем все интерфейсы — работа по IP без домена
	log.Printf("\u26a1 Todorio %s запущен на %s (https=%v)", version, addr, cfg.HTTPS)
	if cfg.HTTPS && cfg.CertFile != "" && cfg.KeyFile != "" {
		return http.ListenAndServeTLS(addr, cfg.CertFile, cfg.KeyFile, handler)
	}
	return http.ListenAndServe(addr, handler)
}

// spaHandler отдаёт файлы из dist, а для клиентских маршрутов (/space/5 и т.п.) — index.html.
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
	// Рядом с бинарником в проде (/usr/share/todorio/migrations), в репо — ./migrations.
	if _, err := os.Stat("/usr/share/todorio/migrations"); err == nil {
		return "/usr/share/todorio/migrations"
	}
	return "migrations"
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
