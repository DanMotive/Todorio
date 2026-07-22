package api

import (
	"net/http"
	"regexp"

	"github.com/DanMotive/Todorio/internal/auth"
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

// POST /api/register — открытая регистрация со статусом pending (ручное одобрение).
// Первый зарегистрированный пользователь становится root (случай dev-запуска без setup).
func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	mode := a.DB.Setting(r.Context(), "policy.registration.mode", "open_approval")
	if mode == "closed" {
		errJSON(w, http.StatusForbidden, "регистрация закрыта")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	if !usernameRe.MatchString(in.Username) {
		errJSON(w, http.StatusBadRequest, "логин: 3–32 символа, только буквы/цифры/_")
		return
	}
	if len(in.Password) < 8 {
		errJSON(w, http.StatusBadRequest, "пароль минимум 8 символов")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка сервера")
		return
	}

	var total int
	_ = a.DB.Pool.QueryRow(r.Context(), `SELECT count(*) FROM users`).Scan(&total)
	role, status := "user", "pending"
	if total == 0 {
		role, status = "root", "active" // dev-бутстрап
	}

	var id int64
	err = a.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO users(username, password_hash, role, status) VALUES($1,$2,$3,$4)
		 ON CONFLICT (username) DO NOTHING RETURNING id`,
		in.Username, hash, role, status).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusConflict, "логин уже занят")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": status})
}

// POST /api/login — вход по логину/паролю; при включённой двухфакторке требуется totp_code.
func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTPCode string `json:"totp_code"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	var (
		id     int64
		hash, role, status string
		mustChange, totpEnabled bool
		totpSecret *string
	)
	err := a.DB.Pool.QueryRow(r.Context(),
		`SELECT id, password_hash, role, status, must_change_password, totp_secret, totp_enabled
		 FROM users WHERE username=$1 AND archived_at IS NULL`,
		in.Username).Scan(&id, &hash, &role, &status, &mustChange, &totpSecret, &totpEnabled)
	if err != nil || !auth.VerifyPassword(in.Password, hash) {
		errJSON(w, http.StatusUnauthorized, "неверный логин или пароль")
		return
	}
	if totpEnabled {
		if in.TOTPCode == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"totp_required": true})
			return
		}
		if totpSecret == nil || !auth.VerifyTOTP(*totpSecret, in.TOTPCode) {
			errJSON(w, http.StatusUnauthorized, "неверный код двухфакторки")
			return
		}
	}
	if status == "blocked" || status == "rejected" {
		errJSON(w, http.StatusForbidden, "доступ отключён администратором")
		return
	}
	if err := auth.CreateSession(r.Context(), a.DB, w, id, r.UserAgent(), a.Cfg.HTTPS); err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка сессии")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "username": in.Username, "role": role, "status": status,
		"must_change_password": mustChange,
	})
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	auth.DestroySession(r.Context(), a.DB, w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/me — доступен и pending-пользователю (страница ожидания).
func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errJSON(w, http.StatusUnauthorized, "требуется вход")
		return
	}
	var unread int
	_ = a.DB.Pool.QueryRow(r.Context(),
		`SELECT count(*) FROM notifications WHERE user_id=$1 AND read_at IS NULL`, u.ID).Scan(&unread)
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "unread_notifications": unread})
}

// PATCH /api/me — локаль, тема, настройки уведомлений, имя.
func (a *API) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errJSON(w, http.StatusUnauthorized, "требуется вход")
		return
	}
	var in struct {
		DisplayName *string `json:"display_name"`
		Locale      *string `json:"locale"`
		ThemeColor  *string `json:"theme_color"`
		ThemeScheme *string `json:"theme_scheme"`
		ThemeVisual *string `json:"theme_visual"`
		NotifyPrefs *map[string]any `json:"notify_prefs"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	_, err := a.DB.Pool.Exec(r.Context(), `
		UPDATE users SET
			display_name = COALESCE($2, display_name),
			locale       = COALESCE($3, locale),
			theme_color  = COALESCE($4, theme_color),
			theme_scheme = COALESCE($5, theme_scheme),
			theme_visual = COALESCE($6, theme_visual)
		WHERE id=$1`,
		u.ID, in.DisplayName, in.Locale, in.ThemeColor, in.ThemeScheme, in.ThemeVisual)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "недопустимое значение (проверь тему: red/blue/green/yellow/gray)")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/me/password — смена пароля (доступна и pending, и must_change_password).
func (a *API) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errJSON(w, http.StatusUnauthorized, "требуется вход")
		return
	}
	var in struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := readJSON(r, &in); err != nil || len(in.NewPassword) < 8 {
		errJSON(w, http.StatusBadRequest, "новый пароль минимум 8 символов")
		return
	}
	var hash string
	if err := a.DB.Pool.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id=$1`, u.ID).Scan(&hash); err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка сервера")
		return
	}
	if !auth.VerifyPassword(in.OldPassword, hash) {
		errJSON(w, http.StatusForbidden, "старый пароль неверен")
		return
	}
	newHash, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка сервера")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(),
		`UPDATE users SET password_hash=$2, must_change_password=false WHERE id=$1`, u.ID, newHash)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
