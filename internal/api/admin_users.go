package api

import (
	"net/http"

	"github.com/DanMotive/Todorio/internal/setup"
)

// GET /api/admin/users?status=pending|active|blocked|rejected (без фильтра — все)
func (a *API) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) == nil {
		return
	}
	status := r.URL.Query().Get("status")
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT id, username, role, status, display_name, created_at, last_seen_at
		FROM users WHERE archived_at IS NULL AND ($1 = '' OR status = $1)
		ORDER BY created_at DESC`, status)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	defer rows.Close()
	users := []map[string]any{}
	for rows.Next() {
		var (
			id int64
			username, role, st string
			displayName *string
			createdAt, lastSeen any
		)
		if err := rows.Scan(&id, &username, &role, &st, &displayName, &createdAt, &lastSeen); err == nil {
			users = append(users, map[string]any{
				"id": id, "username": username, "role": role, "status": st,
				"display_name": displayName, "created_at": createdAt, "last_seen_at": lastSeen,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

// POST /api/admin/users/{id}/approve — одобрение: роль + точечные разрешения.
func (a *API) handleApproveUser(w http.ResponseWriter, r *http.Request) {
	admin := a.requireAdmin(w, r)
	if admin == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный id")
		return
	}
	var in struct {
		Role        string         `json:"role"` // user | viewer | admin (admin назначает только root)
		Permissions map[string]any `json:"permissions"`
	}
	if err := readJSON(r, &in); err != nil || in.Role == "" {
		in.Role = "user"
	}
	if in.Role == "admin" && admin.Role != "root" {
		errJSON(w, http.StatusForbidden, "назначать админов может только root")
		return
	}
	if in.Role == "root" {
		errJSON(w, http.StatusForbidden, "root ровно один")
		return
	}
	_, err = a.DB.Pool.Exec(r.Context(),
		`UPDATE users SET status='active', role=$2 WHERE id=$1 AND status='pending'`, id, in.Role)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	a.notify(r, id, "approved", map[string]any{"role": in.Role})
	// демо-пространство, личное пространство, auto_apply-шаблоны и онбординг-квесты
	var username string
	_ = a.DB.Pool.QueryRow(r.Context(), `SELECT username FROM users WHERE id=$1`, id).Scan(&username)
	a.postApprove(r.Context(), id, username)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/admin/users/{id}/status {status: blocked|rejected|active}
// При блокировке задачи пользователя становятся неназначенными (по ТЗ).
func (a *API) handleSetUserStatus(w http.ResponseWriter, r *http.Request) {
	admin := a.requireAdmin(w, r)
	if admin == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный id")
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	switch in.Status {
	case "blocked", "rejected", "active":
	default:
		errJSON(w, http.StatusBadRequest, "допустимо: blocked | rejected | active")
		return
	}
	var targetRole string
	if err := a.DB.Pool.QueryRow(r.Context(), `SELECT role FROM users WHERE id=$1`, id).Scan(&targetRole); err != nil {
		errJSON(w, http.StatusNotFound, "пользователь не найден")
		return
	}
	if targetRole == "root" {
		errJSON(w, http.StatusForbidden, "root нельзя заблокировать через веб")
		return
	}
	if targetRole == "admin" && admin.Role != "root" {
		errJSON(w, http.StatusForbidden, "админов управляет только root")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE users SET status=$2 WHERE id=$1`, id, in.Status)
	if in.Status == "blocked" || in.Status == "rejected" {
		_, _ = a.DB.Pool.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id)
		_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE tasks SET assignee_id=NULL WHERE assignee_id=$1 AND completed_at IS NULL`, id)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/admin/users/{id}/reset-password — генерирует временный пароль (без писем — админ передаёт лично).
func (a *API) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный id")
		return
	}
	temp, err := setup.GeneratePassword()
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка генерации")
		return
	}
	hash, err := authHash(temp)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка сервера")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(),
		`UPDATE users SET password_hash=$2, must_change_password=true WHERE id=$1`, id, hash)
	_, _ = a.DB.Pool.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id)
	// Временный пароль показывается админу один раз, в логи не пишется.
	writeJSON(w, http.StatusOK, map[string]string{"temp_password": temp})
}
