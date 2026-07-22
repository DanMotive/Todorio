// Package api: Todorio's HTTP handlers (no public API — this is only for our own frontend,
// auth via cookie sessions).
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/DanMotive/Todorio/internal/auth"
	"github.com/DanMotive/Todorio/internal/config"
	"github.com/DanMotive/Todorio/internal/db"
	"github.com/DanMotive/Todorio/internal/events"
)

// Fixed set of reactions from the spec.
var AllowedReactions = map[string]bool{
	"\U0001F44D": true, "\u2705": true, "\U0001F389": true, "\U0001F525": true, "\U0001F440": true,
	"\u2753": true, "\u2757": true, "\u274C": true, "\U0001F62D": true, "\u2B50": true,
}

type API struct {
	DB      *db.DB
	Bus     *events.Bus
	Cfg     config.Config
	Version string
}

func (a *API) Routes(mux *http.ServeMux) {
	// --- auth and profile ---
	mux.HandleFunc("POST /api/register", a.handleRegister)
	mux.HandleFunc("POST /api/login", a.handleLogin)
	mux.HandleFunc("POST /api/logout", a.handleLogout)
	mux.HandleFunc("GET /api/me", a.handleMe)
	mux.HandleFunc("PATCH /api/me", a.handleUpdateMe)
	mux.HandleFunc("POST /api/me/password", a.handleChangePassword)

	// --- user administration ---
	mux.HandleFunc("GET /api/admin/users", a.handleAdminUsers)
	mux.HandleFunc("POST /api/admin/users/{id}/approve", a.handleApproveUser)
	mux.HandleFunc("POST /api/admin/users/{id}/status", a.handleSetUserStatus)
	mux.HandleFunc("POST /api/admin/users/{id}/reset-password", a.handleResetPassword)

	// --- spaces and lists ---
	mux.HandleFunc("GET /api/spaces", a.handleListSpaces)
	mux.HandleFunc("POST /api/spaces", a.handleCreateSpace)
	mux.HandleFunc("PATCH /api/spaces/{id}", a.handleUpdateSpace)
	mux.HandleFunc("DELETE /api/spaces/{id}", a.handleArchiveSpace)
	mux.HandleFunc("POST /api/spaces/{id}/members", a.handleAddSpaceMember)
	mux.HandleFunc("GET /api/spaces/{id}/lists", a.handleListLists)
	mux.HandleFunc("POST /api/spaces/{id}/lists", a.handleCreateList)
	mux.HandleFunc("PATCH /api/lists/{id}", a.handleUpdateList)
	mux.HandleFunc("DELETE /api/lists/{id}", a.handleArchiveList)
	mux.HandleFunc("POST /api/lists/{id}/members", a.handleAddListMember)
	mux.HandleFunc("GET /api/lists/{id}/share", a.handleListShareLinks)
	mux.HandleFunc("POST /api/lists/{id}/share", a.handleCreateShareLink)
	mux.HandleFunc("DELETE /api/shares/{id}", a.handleRevokeShareLink)
	mux.HandleFunc("GET /api/public/{token}", a.handlePublicShare)

	// --- tasks ---
	mux.HandleFunc("GET /api/lists/{id}/tasks", a.handleListTasks)
	mux.HandleFunc("POST /api/lists/{id}/tasks", a.handleCreateTask)
	mux.HandleFunc("GET /api/tasks/{id}", a.handleGetTask)
	mux.HandleFunc("PATCH /api/tasks/{id}", a.handleUpdateTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", a.handleArchiveTask)
	mux.HandleFunc("GET /api/my/tasks", a.handleMyTasks)

	// --- social interactions ---
	mux.HandleFunc("GET /api/tasks/{id}/comments", a.handleListComments)
	mux.HandleFunc("POST /api/tasks/{id}/comments", a.handleCreateComment)
	mux.HandleFunc("DELETE /api/comments/{id}", a.handleDeleteComment)
	mux.HandleFunc("POST /api/reactions", a.handleToggleReaction)

	// --- notifications and realtime ---
	mux.HandleFunc("GET /api/notifications", a.handleListNotifications)
	mux.HandleFunc("POST /api/notifications/read", a.handleReadNotifications)
	mux.HandleFunc("GET /api/events", a.handleSSE)

	// --- space Pulse and stats ---
	mux.HandleFunc("GET /api/spaces/{id}/pulse", a.handlePulse)
	mux.HandleFunc("GET /api/spaces/{id}/stats", a.handleStats)

	// --- TOTP (2FA for root/admins) ---
	mux.HandleFunc("POST /api/me/totp/setup", a.handleTOTPSetup)
	mux.HandleFunc("POST /api/me/totp/enable", a.handleTOTPEnable)
	mux.HandleFunc("POST /api/me/totp/disable", a.handleTOTPDisable)

	// --- image attachments ---
	mux.HandleFunc("POST /api/tasks/{id}/attachments", a.handleUploadAttachment)
	mux.HandleFunc("GET /api/tasks/{id}/attachments", a.handleListAttachments)
	mux.HandleFunc("GET /api/attachments/{id}", a.handleGetAttachment)
	mux.HandleFunc("DELETE /api/attachments/{id}", a.handleDeleteAttachment)

	// --- announcements ---
	mux.HandleFunc("POST /api/announcements", a.handleCreateAnnouncement)
	mux.HandleFunc("GET /api/announcements/active", a.handleActiveAnnouncements)
	mux.HandleFunc("POST /api/announcements/{id}/ack", a.handleAckAnnouncement)

	// --- list templates ---
	mux.HandleFunc("POST /api/admin/templates", a.handleCreateTemplate)
	mux.HandleFunc("DELETE /api/admin/templates/{id}", a.handleDeleteTemplate)
	mux.HandleFunc("GET /api/templates", a.handleListTemplates)
	mux.HandleFunc("POST /api/templates/{id}/apply", a.handleApplyTemplate)

	// --- "while you were away" digest ---
	mux.HandleFunc("GET /api/digest", a.handleDigest)
	mux.HandleFunc("POST /api/digest/dismiss", a.handleDigestDismiss)

	// --- custom workflow statuses ---
	mux.HandleFunc("GET /api/spaces/{id}/workflow", a.handleGetWorkflow)

	// --- notes ---
	mux.HandleFunc("GET /api/spaces/{id}/notes", a.handleListNotes)
	mux.HandleFunc("POST /api/spaces/{id}/notes", a.handleCreateNote)
	mux.HandleFunc("GET /api/notes/{id}", a.handleGetNote)
	mux.HandleFunc("PATCH /api/notes/{id}", a.handleUpdateNote)
	mux.HandleFunc("DELETE /api/notes/{id}", a.handleArchiveNote)

	// --- favorites ---
	mux.HandleFunc("GET /api/favorites", a.handleListFavorites)
	mux.HandleFunc("POST /api/favorites", a.handleToggleFavorite)

	// --- saved filters ---
	mux.HandleFunc("GET /api/filters", a.handleListFilters)
	mux.HandleFunc("POST /api/filters", a.handleCreateFilter)
	mux.HandleFunc("DELETE /api/filters/{id}", a.handleDeleteFilter)

	// --- global search ---
	mux.HandleFunc("GET /api/search", a.handleSearch)

	// --- focus mode ---
	mux.HandleFunc("POST /api/focus/start", a.handleStartFocus)
	mux.HandleFunc("POST /api/focus/stop", a.handleStopFocus)
	mux.HandleFunc("GET /api/focus/stats", a.handleFocusStats)

	// --- activity feed ---
	mux.HandleFunc("GET /api/spaces/{id}/activity", a.handleSpaceActivity)

	// --- custom fields ---
	mux.HandleFunc("GET /api/spaces/{id}/fields", a.handleGetFields)
	mux.HandleFunc("PUT /api/spaces/{id}/fields", a.handleSetFields)
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// requireUser — active users only. A pending user can only reach /api/me and password change.
func (a *API) requireUser(w http.ResponseWriter, r *http.Request) *auth.User {
	u := auth.FromContext(r.Context())
	if u == nil {
		errJSON(w, http.StatusUnauthorized, "login required")
		return nil
	}
	if u.Status != "active" {
		errJSON(w, http.StatusForbidden, "account is pending approval or blocked")
		return nil
	}
	return u
}

func (a *API) requireAdmin(w http.ResponseWriter, r *http.Request) *auth.User {
	u := a.requireUser(w, r)
	if u == nil {
		return nil
	}
	if !u.IsAdmin() {
		errJSON(w, http.StatusForbidden, "administrator permission required")
		return nil
	}
	return u
}

// listPermission returns the user's permission on a list: owner | editor | viewer | "" (no access).
// Root/admin get owner.
func (a *API) listPermission(r *http.Request, u *auth.User, listID int64) string {
	if u.IsAdmin() {
		return "owner"
	}
	var perm string
	err := a.DB.Pool.QueryRow(r.Context(),
		`SELECT permission FROM list_members WHERE list_id=$1 AND user_id=$2`, listID, u.ID).Scan(&perm)
	if err != nil {
		return ""
	}
	return perm
}

func permAtLeast(perm, min string) bool {
	rank := map[string]int{"": 0, "viewer": 1, "editor": 2, "owner": 3}
	return rank[perm] >= rank[min]
}

// notify creates a notification in the DB and publishes it over SSE, unless the recipient is in
// a Do Not Disturb window — the notification is still saved and delivered the next time the user
// opens notifications or the "while you were away" digest.
func (a *API) notify(r *http.Request, userID int64, kind string, payload map[string]any) {
	b, _ := json.Marshal(payload)
	_, _ = a.DB.Pool.Exec(r.Context(),
		`INSERT INTO notifications(user_id, kind, payload) VALUES($1,$2,$3)`, userID, kind, string(b))
	if a.inDoNotDisturb(r.Context(), userID) {
		return
	}
	a.Bus.Publish([]int64{userID}, events.Event{Type: "notification", Data: map[string]any{"kind": kind, "payload": payload}})
}

// inDoNotDisturb reports whether the user's quiet-hours window covers the current server time.
// Format: users.notify_prefs -> {"dnd": {"enabled": true, "start": "22:00", "end": "08:00"}}.
func (a *API) inDoNotDisturb(ctx context.Context, userID int64) bool {
	var raw *string
	if a.DB.Pool.QueryRow(ctx, `SELECT notify_prefs #>> '{dnd}' FROM users WHERE id=$1`, userID).Scan(&raw) != nil || raw == nil {
		return false
	}
	var dnd struct {
		Enabled bool   `json:"enabled"`
		Start   string `json:"start"`
		End     string `json:"end"`
	}
	if json.Unmarshal([]byte(*raw), &dnd) != nil || !dnd.Enabled {
		return false
	}
	start, err1 := time.Parse("15:04", dnd.Start)
	end, err2 := time.Parse("15:04", dnd.End)
	if err1 != nil || err2 != nil {
		return false
	}
	now := time.Now()
	nowMin := now.Hour()*60 + now.Minute()
	startMin := start.Hour()*60 + start.Minute()
	endMin := end.Hour()*60 + end.Minute()
	if startMin == endMin {
		return false
	}
	if startMin < endMin {
		return nowMin >= startMin && nowMin < endMin
	}
	return nowMin >= startMin || nowMin < endMin
}
