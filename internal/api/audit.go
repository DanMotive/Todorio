package api

// Audit trail for administrative actions (migration 0013).
//
// Approving a user, changing a role, blocking an account, resetting a password, permanently
// deleting a task/list/space and changing a server policy all left no trace. With more than one
// admin in a workspace, "who blocked this account and when" had no answer at all.
//
// Two rules this file follows:
//
//   - Recording is best-effort and never fails the action. An admin blocking an abusive account
//     must not be stopped because the log table is unavailable; a failure is logged instead.
//   - Nothing secret is ever written. Password resets record that a reset happened, never the
//     generated password; setting changes record the key and the new value only for keys that
//     are not credentials.

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/DanMotive/Todorio/internal/auth"
)

// Action names. Constants rather than inline strings so a rename cannot silently split one
// action into two in the history.
const (
	auditUserApprove       = "user.approve"
	auditUserStatus        = "user.status"
	auditUserResetPassword = "user.reset_password"
	auditTaskPurge         = "task.delete_permanent"
	auditListPurge         = "list.delete_permanent"
	auditSpacePurge        = "space.delete_permanent"
	auditSettingChange     = "setting.change"
	auditLocaleToggle      = "locale.toggle"
)

// auditRedactedKeys are settings whose value must never reach the log. The fact that the key
// changed is worth recording; the value is a credential.
var auditRedactedKeys = map[string]bool{
	"telegram.bot_token": true,
}

// audit records one administrative action. targetID may be 0 when the action has no numeric
// target (a setting key, for instance), in which case the column stays NULL.
func (a *API) audit(r *http.Request, actor *auth.User, action, targetType string, targetID int64, details map[string]any) {
	if actor == nil {
		return
	}
	if details == nil {
		details = map[string]any{}
	}
	payload, err := json.Marshal(details)
	if err != nil {
		payload = []byte(`{}`)
	}
	var target any
	if targetID != 0 {
		target = targetID
	}
	// actor.Username is stored as a snapshot: the trail has to stay readable after the account
	// is renamed or deleted, and the foreign key is ON DELETE SET NULL.
	_, err = a.DB.Pool.Exec(r.Context(), `
		INSERT INTO admin_audit(actor_id, actor_username, action, target_type, target_id, details, ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		actor.ID, actor.Username, action, targetType, target, string(payload), clientIP(r))
	if err != nil {
		// Deliberately not surfaced to the caller — see the file comment.
		log.Printf("audit: could not record %q by %s: %v", action, actor.Username, err)
	}
}

// GET /api/admin/audit?limit=&actor=&action= — newest first.
func (a *API) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) == nil {
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	actor := int64(0)
	if v := r.URL.Query().Get("actor"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			actor = n
		}
	}
	action := r.URL.Query().Get("action")

	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT id, actor_id, actor_username, action, target_type, target_id, details, ip, created_at
		FROM admin_audit
		WHERE ($1 = 0 OR actor_id = $1) AND ($2 = '' OR action = $2)
		ORDER BY created_at DESC, id DESC
		LIMIT $3`, actor, action, limit)
	if err != nil {
		dbFail(r, "audit list", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	entries := []map[string]any{}
	for rows.Next() {
		var (
			id                                 int64
			actorID, targetID                  *int64
			actorUsername, act, targetType, ip string
			details                            []byte
			createdAt                          any
		)
		if rows.Scan(&id, &actorID, &actorUsername, &act, &targetType, &targetID, &details, &ip, &createdAt) != nil {
			continue
		}
		var parsed any
		if json.Unmarshal(details, &parsed) != nil {
			parsed = map[string]any{}
		}
		entries = append(entries, map[string]any{
			"id": id, "actor_id": actorID, "actor_username": actorUsername,
			"action": act, "target_type": targetType, "target_id": targetID,
			"details": parsed, "ip": ip, "created_at": createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
