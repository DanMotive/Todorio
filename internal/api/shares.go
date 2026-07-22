package api

// Public read-only share links: /s/{token}, served without authentication.
// Enabled/disabled globally via policy.sharing.public_links (default true). Optional password
// protection (password_hash) can be set when creating the link.

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/DanMotive/Todorio/internal/auth"
)

func randomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GET /api/lists/{id}/share — active share links for a list (list owner only).
func (a *API) handleListShareLinks(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	listID, err := pathID(r)
	if err != nil || !permAtLeast(a.listPermission(r, u, listID), "owner") {
		errJSON(w, http.StatusForbidden, "list owner permission required")
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT id, token, expires_at, (password_hash IS NOT NULL) AS has_password, created_at
		FROM share_links WHERE list_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC`, listID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	defer rows.Close()
	links := []map[string]any{}
	for rows.Next() {
		var id int64
		var token string
		var expiresAt *time.Time
		var hasPassword bool
		var createdAt time.Time
		if rows.Scan(&id, &token, &expiresAt, &hasPassword, &createdAt) == nil {
			links = append(links, map[string]any{
				"id": id, "token": token, "expires_at": expiresAt, "has_password": hasPassword, "created_at": createdAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": links})
}

// POST /api/lists/{id}/share {expires_in_days?, password?} — list owner only.
func (a *API) handleCreateShareLink(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if a.DB.Setting(r.Context(), "policy.sharing.public_links", "true") != "true" {
		errJSON(w, http.StatusForbidden, "public links are disabled by the administrator")
		return
	}
	listID, err := pathID(r)
	if err != nil || !permAtLeast(a.listPermission(r, u, listID), "owner") {
		errJSON(w, http.StatusForbidden, "list owner permission required")
		return
	}
	var in struct {
		ExpiresInDays *int    `json:"expires_in_days"`
		Password      *string `json:"password"`
	}
	_ = readJSON(r, &in)
	var expiresAt *time.Time
	if in.ExpiresInDays != nil && *in.ExpiresInDays > 0 {
		t := time.Now().AddDate(0, 0, *in.ExpiresInDays)
		expiresAt = &t
	}
	var passwordHash *string
	if in.Password != nil && *in.Password != "" {
		h, err := auth.HashPassword(*in.Password)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "could not hash the password")
			return
		}
		passwordHash = &h
	}
	token := randomToken()
	var id int64
	err = a.DB.Pool.QueryRow(r.Context(), `
		INSERT INTO share_links(token, list_id, created_by, expires_at, password_hash)
		VALUES($1,$2,$3,$4,$5) RETURNING id`,
		token, listID, u.ID, expiresAt, passwordHash).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "token": token})
}

// DELETE /api/shares/{id} — revoke a share link (list owner only).
func (a *API) handleRevokeShareLink(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var listID int64
	if a.DB.Pool.QueryRow(r.Context(), `SELECT list_id FROM share_links WHERE id=$1`, id).Scan(&listID) != nil {
		errJSON(w, http.StatusNotFound, "share link not found")
		return
	}
	if !permAtLeast(a.listPermission(r, u, listID), "owner") {
		errJSON(w, http.StatusForbidden, "list owner permission required")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE share_links SET revoked_at=now() WHERE id=$1`, id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/public/{token}?password= — unauthenticated read-only view of a shared list.
func (a *API) handlePublicShare(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	var listID int64
	var expiresAt *time.Time
	var revokedAt *time.Time
	var passwordHash *string
	err := a.DB.Pool.QueryRow(r.Context(),
		`SELECT list_id, expires_at, revoked_at, password_hash FROM share_links WHERE token=$1`, token).
		Scan(&listID, &expiresAt, &revokedAt, &passwordHash)
	if err != nil {
		errJSON(w, http.StatusNotFound, "link not found")
		return
	}
	if revokedAt != nil || (expiresAt != nil && expiresAt.Before(time.Now())) {
		errJSON(w, http.StatusGone, "this link has expired or was revoked")
		return
	}
	if passwordHash != nil {
		password := r.URL.Query().Get("password")
		if password == "" || !auth.VerifyPassword(password, *passwordHash) {
			errJSON(w, http.StatusUnauthorized, "password required")
			return
		}
	}
	var listName string
	if a.DB.Pool.QueryRow(r.Context(), `SELECT name FROM lists WHERE id=$1 AND archived_at IS NULL`, listID).Scan(&listName) != nil {
		errJSON(w, http.StatusNotFound, "list not found")
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT id, title, due_at, completed_at FROM tasks
		WHERE list_id=$1 AND archived_at IS NULL ORDER BY position, id`, listID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	defer rows.Close()
	tasks := []map[string]any{}
	for rows.Next() {
		var id int64
		var title string
		var dueAt, completedAt *time.Time
		if rows.Scan(&id, &title, &dueAt, &completedAt) == nil {
			tasks = append(tasks, map[string]any{"id": id, "title": title, "due_at": dueAt, "completed_at": completedAt})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"list": map[string]any{"name": listName}, "tasks": tasks})
}
