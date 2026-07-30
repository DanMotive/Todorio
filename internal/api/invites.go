package api

// Invite codes: an alternative on-ramp to manual approval (spec section 4/21 explicitly rules out
// email invites — these are short codes shared out-of-band, e.g. over chat). Redeeming a valid code
// during registration (see handleRegister in auth_handlers.go) activates the account immediately,
// skipping the pending queue. Root/admins can always create and manage invites; regular active users
// can create them too, but only when policy.users.can_invite is enabled (default: off).

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"
)

type inviteRow struct {
	ID   int64
	Role string
}

// lookupInvite resolves a code to a still-usable invite: exists, not expired, uses remaining.
func (a *API) lookupInvite(ctx context.Context, code string) (*inviteRow, error) {
	var iv inviteRow
	var expiresAt *time.Time
	var usedCount, maxUses int
	err := a.DB.Pool.QueryRow(ctx,
		`SELECT id, role, max_uses, used_count, expires_at FROM invites WHERE code=$1`, code).
		Scan(&iv.ID, &iv.Role, &maxUses, &usedCount, &expiresAt)
	if err != nil {
		return nil, errors.New("invite not found")
	}
	if usedCount >= maxUses {
		return nil, errors.New("invite already used up")
	}
	if expiresAt != nil && expiresAt.Before(time.Now()) {
		return nil, errors.New("invite expired")
	}
	return &iv, nil
}

// generateInviteCode returns a 12-character hex code — short enough to read aloud or paste in chat.
func generateInviteCode() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GET /api/invites — admin only.
func (a *API) handleListInvites(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) == nil {
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT i.id, i.code, i.role, i.max_uses, i.used_count, i.expires_at, u.username
		FROM invites i JOIN users u ON u.id = i.created_by
		ORDER BY i.created_at DESC`)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var code, role, createdBy string
		var maxUses, usedCount int
		var expiresAt *time.Time
		if rows.Scan(&id, &code, &role, &maxUses, &usedCount, &expiresAt, &createdBy) == nil {
			list = append(list, map[string]any{
				"id": id, "code": code, "role": role, "max_uses": maxUses,
				"used_count": usedCount, "expires_at": expiresAt, "created_by": createdBy,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": list})
}

// POST /api/invites {max_uses?, expires_days?, role?} — admins always; regular active users only
// when policy.users.can_invite is enabled.
func (a *API) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if !u.IsAdmin() && a.DB.Setting(r.Context(), "policy.users.can_invite", "false") != "true" {
		errJSON(w, http.StatusForbidden, "invites are disabled for regular users")
		return
	}
	var in struct {
		MaxUses     *int    `json:"max_uses"`
		ExpiresDays *int    `json:"expires_days"`
		Role        *string `json:"role"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	maxUses := 1
	if in.MaxUses != nil && *in.MaxUses >= 1 && *in.MaxUses <= 100 {
		maxUses = *in.MaxUses
	}
	role := "user"
	if in.Role != nil {
		switch *in.Role {
		case "user", "viewer":
			role = *in.Role
		case "admin":
			if u.Role == "root" {
				role = "admin"
			}
		}
	}
	var expiresAt *time.Time
	if in.ExpiresDays != nil && *in.ExpiresDays > 0 {
		t := time.Now().AddDate(0, 0, *in.ExpiresDays)
		expiresAt = &t
	}
	code, err := generateInviteCode()
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "server error")
		return
	}
	var id int64
	if err := a.DB.Pool.QueryRow(r.Context(), `
		INSERT INTO invites(code, created_by, role, max_uses, expires_at) VALUES($1,$2,$3,$4,$5) RETURNING id`,
		code, u.ID, role, maxUses, expiresAt).Scan(&id); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "code": code})
}

// DELETE /api/invites/{id} — admin only (any admin can revoke any invite, matching how the rest of
// the admin panel has no per-creator ownership split).
func (a *API) handleDeleteInvite(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `DELETE FROM invites WHERE id=$1`, id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
