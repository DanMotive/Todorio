package api

import (
	"errors"
	"net/http"

	"github.com/DanMotive/Todorio/internal/setup"
	"github.com/jackc/pgx/v5"
)

// GET /api/admin/users?status=pending|active|blocked|rejected (no filter = all)
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
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	users := []map[string]any{}
	for rows.Next() {
		var (
			id                  int64
			username, role, st  string
			displayName         *string
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

// POST /api/admin/users/{id}/approve — approval: role + fine-grained permissions.
func (a *API) handleApproveUser(w http.ResponseWriter, r *http.Request) {
	admin := a.requireAdmin(w, r)
	if admin == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in struct {
		Role        string         `json:"role"` // user | viewer | admin (only root can assign admin)
		Permissions map[string]any `json:"permissions"`
	}
	in.Role = "user"
	if r.ContentLength > 0 {
		if err := readJSON(r, &in); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request")
			return
		}
	}
	switch in.Role {
	case "user", "viewer":
	case "admin":
		if admin.Role != "root" {
			errJSON(w, http.StatusForbidden, "only root can assign admins")
			return
		}
	default:
		errJSON(w, http.StatusBadRequest, "allowed roles: user | viewer | admin")
		return
	}
	if in.Permissions == nil {
		in.Permissions = map[string]any{}
	}

	// The conditional UPDATE is the idempotency boundary: onboarding must run exactly once and
	// only for a real pending account. A repeated click or stale admin tab receives 409 instead
	// of creating another personal space and another set of onboarding tasks.
	var username string
	err = a.DB.Pool.QueryRow(r.Context(), `
		UPDATE users SET status='active', role=$2, permissions=$3
		WHERE id=$1 AND status='pending'
		RETURNING username`, id, in.Role, in.Permissions).Scan(&username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errJSON(w, http.StatusConflict, "user not found or no longer pending")
			return
		}
		dbFail(r, "approve user", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	a.notify(r, id, "approved", map[string]any{"role": in.Role})
	// demo space, personal space, auto_apply templates, and onboarding quests
	a.postApprove(r.Context(), id, username, in.Role)
	a.audit(r, admin, auditUserApprove, "user", id, map[string]any{"username": username, "role": in.Role})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/admin/users/{id}/status {status: blocked|rejected|active}
// When a user is blocked, their tasks become unassigned (per spec).
func (a *API) handleSetUserStatus(w http.ResponseWriter, r *http.Request) {
	admin := a.requireAdmin(w, r)
	if admin == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	switch in.Status {
	case "blocked", "rejected", "active":
	default:
		errJSON(w, http.StatusBadRequest, "allowed values: blocked | rejected | active")
		return
	}
	var targetRole string
	if err := a.DB.Pool.QueryRow(r.Context(), `SELECT role FROM users WHERE id=$1`, id).Scan(&targetRole); err != nil {
		errJSON(w, http.StatusNotFound, "user not found")
		return
	}
	if targetRole == "root" {
		errJSON(w, http.StatusForbidden, "root cannot be blocked via the web")
		return
	}
	if targetRole == "admin" && admin.Role != "root" {
		errJSON(w, http.StatusForbidden, "only root can manage admins")
		return
	}
	tx, err := a.DB.Pool.Begin(r.Context())
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if _, err = tx.Exec(r.Context(), `UPDATE users SET status=$2 WHERE id=$1`, id, in.Status); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if in.Status == "blocked" || in.Status == "rejected" {
		if _, err = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id); err != nil {
			errJSON(w, http.StatusInternalServerError, "database error")
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE tasks SET assignee_id=NULL WHERE assignee_id=$1 AND completed_at IS NULL`, id); err != nil {
			errJSON(w, http.StatusInternalServerError, "database error")
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	a.audit(r, admin, auditUserStatus, "user", id, map[string]any{"status": in.Status, "target_role": targetRole})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/admin/users/{id}/reset-password — generates a temporary password (no emails — the admin shares it in person).
func (a *API) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	admin := a.requireAdmin(w, r)
	if admin == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var targetRole string
	if err := a.DB.Pool.QueryRow(r.Context(), `SELECT role FROM users WHERE id=$1 AND archived_at IS NULL`, id).Scan(&targetRole); err != nil {
		errJSON(w, http.StatusNotFound, "user not found")
		return
	}
	if targetRole == "root" {
		errJSON(w, http.StatusForbidden, "root password cannot be reset via the web")
		return
	}
	if targetRole == "admin" && admin.Role != "root" {
		errJSON(w, http.StatusForbidden, "only root can manage admins")
		return
	}
	temp, err := setup.GeneratePassword()
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "generation error")
		return
	}
	hash, err := authHash(temp)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "server error")
		return
	}
	tx, err := a.DB.Pool.Begin(r.Context())
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	tag, err := tx.Exec(r.Context(),
		`UPDATE users SET password_hash=$2, must_change_password=true WHERE id=$1`, id, hash)
	if err != nil || tag.RowsAffected() != 1 {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	// The temporary password is shown to the admin once and never written to logs — including
	// the audit trail, which records only that a reset happened.
	a.audit(r, admin, auditUserResetPassword, "user", id, map[string]any{"target_role": targetRole})
	writeJSON(w, http.StatusOK, map[string]string{"temp_password": temp})
}
