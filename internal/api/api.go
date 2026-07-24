// Package api: Todorio's HTTP handlers (no public API — this is only for our own frontend,
// auth via cookie sessions).
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
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
	mux.HandleFunc("POST /api/me/avatar", a.handleUploadAvatar)
	mux.HandleFunc("DELETE /api/me/avatar", a.handleDeleteAvatar)
	mux.HandleFunc("GET /api/users/{id}/avatar", a.handleGetAvatar)

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
	mux.HandleFunc("POST /api/spaces/{id}/restore", a.handleRestoreSpace)
	mux.HandleFunc("DELETE /api/spaces/{id}/permanent", a.handleDeleteSpacePermanent)
	mux.HandleFunc("GET /api/spaces/{id}/archive", a.handleSpaceArchive)
	mux.HandleFunc("GET /api/archive/spaces", a.handleArchivedSpaces)
	mux.HandleFunc("POST /api/spaces/{id}/members", a.handleAddSpaceMember)
	mux.HandleFunc("GET /api/spaces/{id}/lists", a.handleListLists)
	mux.HandleFunc("POST /api/spaces/{id}/lists", a.handleCreateList)
	mux.HandleFunc("PATCH /api/lists/{id}", a.handleUpdateList)
	mux.HandleFunc("DELETE /api/lists/{id}", a.handleArchiveList)
	mux.HandleFunc("POST /api/lists/{id}/restore", a.handleRestoreList)
	mux.HandleFunc("DELETE /api/lists/{id}/permanent", a.handleDeleteListPermanent)
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
	mux.HandleFunc("POST /api/tasks/{id}/restore", a.handleRestoreTask)
	mux.HandleFunc("DELETE /api/tasks/{id}/permanent", a.handleDeleteTaskPermanent)
	mux.HandleFunc("GET /api/tasks/{id}/versions", a.handleListTaskVersions)
	mux.HandleFunc("POST /api/tasks/{id}/versions/{version_id}/restore", a.handleRestoreTaskVersion)
	mux.HandleFunc("GET /api/my/tasks", a.handleMyTasks)
	mux.HandleFunc("GET /api/inbox", a.handleInbox)
	mux.HandleFunc("GET /api/my/stats", a.handleMyStats)

	// --- social interactions ---
	mux.HandleFunc("GET /api/tasks/{id}/comments", a.handleListComments)
	mux.HandleFunc("POST /api/tasks/{id}/comments", a.handleCreateComment)
	mux.HandleFunc("PATCH /api/comments/{id}", a.handleUpdateComment)
	mux.HandleFunc("DELETE /api/comments/{id}", a.handleDeleteComment)
	mux.HandleFunc("POST /api/reactions", a.handleToggleReaction)

	// --- notifications and realtime ---
	mux.HandleFunc("GET /api/notifications", a.handleListNotifications)
	mux.HandleFunc("POST /api/notifications/read", a.handleReadNotifications)
	mux.HandleFunc("GET /api/events", a.handleSSE)

	// --- space Pulse and stats ---
	mux.HandleFunc("GET /api/spaces/{id}/pulse", a.handlePulse)
	mux.HandleFunc("GET /api/spaces/{id}/stats", a.handleStats)
	mux.HandleFunc("GET /api/spaces/{id}/timeline", a.handleTimeline)

	// --- TOTP (2FA for root/admins) ---
	mux.HandleFunc("POST /api/me/totp/setup", a.handleTOTPSetup)
	mux.HandleFunc("POST /api/me/totp/enable", a.handleTOTPEnable)
	mux.HandleFunc("POST /api/me/totp/disable", a.handleTOTPDisable)

	// --- image attachments ---
	mux.HandleFunc("POST /api/tasks/{id}/attachments", a.handleUploadAttachment)
	mux.HandleFunc("GET /api/tasks/{id}/attachments", a.handleListAttachments)
	mux.HandleFunc("POST /api/comments/{id}/attachments", a.handleUploadCommentAttachment)
	mux.HandleFunc("GET /api/comments/{id}/attachments", a.handleListCommentAttachments)

	// --- instance logo (spec section 18) ---
	mux.HandleFunc("POST /api/admin/logo", a.handleUploadLogo)
	mux.HandleFunc("DELETE /api/admin/logo", a.handleDeleteLogo)
	mux.HandleFunc("GET /api/logo", a.handleGetLogo)
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

	// --- invite codes ---
	mux.HandleFunc("GET /api/invites", a.handleListInvites)
	mux.HandleFunc("POST /api/invites", a.handleCreateInvite)
	mux.HandleFunc("DELETE /api/invites/{id}", a.handleDeleteInvite)

	// --- server settings (root panel + CLI share the same system_settings table) ---
	mux.HandleFunc("GET /api/admin/settings", a.handleGetSettings)
	mux.HandleFunc("POST /api/admin/settings", a.handleSetSetting)
	mux.HandleFunc("POST /api/admin/locales", a.handleSetLocale)
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

// dbFail logs the real database error to the server log before the handler replies with a
// deliberately vague "database error" to the client. The client message stays vague on
// purpose (no schema details leak to the browser), but a blind 500 is undiagnosable when a
// user reports a failure — the server log is where the actual cause has to be visible.
// op should read like "create task" so the log line says which handler failed.
func dbFail(r *http.Request, op string, err error) {
	if err == nil {
		return
	}
	log.Printf("db error: %s: %v [%s %s]", op, err, r.Method, r.URL.Path)
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// pathIDNamed reads a specific {name} path parameter — for routes with more than one ID segment,
// e.g. /api/tasks/{id}/versions/{version_id}/restore, where plain pathID's hardcoded "id" only
// gets the first one.
func pathIDNamed(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(r.PathValue(name), 10, 64)
}

// intSetting reads a numeric limit setting. Per spec section 10, "0 = no limit" is a real,
// deliberate admin choice distinct from "not configured" — a naive `strconv.Atoi` + `if n > 0`
// check (the pattern this replaces at every limit call site) can't tell those apart and silently
// falls back to a hardcoded default for both, so an admin explicitly setting 0 never actually got
// "unlimited". This returns `def` only when the key has no stored value or an unparseable one;
// an explicitly stored 0 is returned as 0, and callers are expected to treat 0 as "no limit".
// enforceSessionLimit evicts the oldest active sessions for userID until creating one more
// session would land exactly at the configured limit (spec section 10 example: "10 sessions per
// user"). Rejecting the login outright would be a worse experience than quietly signing the
// oldest device out — the same tradeoff most apps with a "log out other devices" limit make.
// limit <= 0 (including the "not configured" default) means unlimited: nothing to do.
func (a *API) enforceSessionLimit(ctx context.Context, userID int64) {
	limit := a.intSetting(ctx, "limits.login.max_sessions_per_user", 0)
	if limit <= 0 {
		return
	}
	var count int
	_ = a.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1 AND expires_at > now()`, userID).Scan(&count)
	if count < limit {
		return
	}
	toEvict := count - limit + 1
	_, _ = a.DB.Pool.Exec(ctx, `
		DELETE FROM sessions WHERE id IN (
			SELECT id FROM sessions WHERE user_id=$1 AND expires_at > now()
			ORDER BY created_at ASC LIMIT $2)`, userID, toEvict)
}

// dirSize walks root and sums the size of every regular file in it — used for the total server
// storage quota below (spec section 10 example: "20 GB total"). A plain recursive walk rather
// than a DB-tracked running counter: a self-hosted instance realistically has at most a few
// thousand uploaded files, so a walk is fast, and unlike a counter it can never drift from what's
// actually on disk (crashes, manual file deletions, or a bug in the bookkeeping can't desync it).
func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip entries we can't stat (e.g. a race with a concurrent delete) instead of failing the whole walk
		}
		if d.IsDir() {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// checkStorageQuota returns a user-facing error if accepting `incoming` more bytes would push the
// whole uploads directory (every task attachment and avatar — everything under cfg.UploadsDir)
// past limits.uploads.max_total_storage_mb. 0 (the default) means unlimited — the whole check is
// skipped, so upgrading never suddenly caps an existing instance.
func (a *API) checkStorageQuota(ctx context.Context, incoming int64) error {
	maxMB := a.intSetting(ctx, "limits.uploads.max_total_storage_mb", 0)
	if maxMB <= 0 {
		return nil
	}
	used, err := dirSize(a.Cfg.UploadsDir)
	if err != nil {
		return nil // fail open: a storage hiccup shouldn't block every upload
	}
	if used+incoming > int64(maxMB)<<20 {
		return fmt.Errorf("server storage limit reached (%d MB total) — ask an admin to free up space or raise the limit", maxMB)
	}
	return nil
}

// featureEnabled reads a global policy.features.<name> on/off toggle (spec section 10: comments,
// reactions, attachments, versions, and stats can each be switched off instance-wide). Every
// feature defaults to enabled — these are opt-out, not opt-in, so upgrading never silently turns
// something off that was already in use.
func (a *API) featureEnabled(ctx context.Context, name string) bool {
	return a.DB.Setting(ctx, "policy.features."+name, "true") != "false"
}

func (a *API) intSetting(ctx context.Context, key string, def int) int {
	raw := a.DB.Setting(ctx, key, "")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
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

// notify creates a notification in the DB and publishes it over SSE, unless the recipient has
// turned this kind off entirely (users.notify_prefs.types.<kind> = false) — in which case nothing
// is recorded at all, per spec section 7 ("each type is switched on/off in the profile"). If the
// type is on but the recipient is in a Do Not Disturb window, the notification is still saved and
// delivered the next time they open notifications or the "while you were away" digest — DND only
// suppresses the live SSE push.
func (a *API) notify(r *http.Request, userID int64, kind string, payload map[string]any) {
	if !a.notifyTypeEnabled(r.Context(), userID, kind) {
		return
	}
	b, _ := json.Marshal(payload)
	_, _ = a.DB.Pool.Exec(r.Context(),
		`INSERT INTO notifications(user_id, kind, payload) VALUES($1,$2,$3)`, userID, kind, string(b))
	if a.inDoNotDisturb(r.Context(), userID) {
		return
	}
	a.Bus.Publish([]int64{userID}, events.Event{Type: "notification", Data: map[string]any{"kind": kind, "payload": payload}})
}

// notifyTypeEnabled reports whether userID wants this notification kind at all (default: on).
// users.notify_prefs -> {"types": {"comment": false, ...}}.
func (a *API) notifyTypeEnabled(ctx context.Context, userID int64, kind string) bool {
	var raw *string
	if a.DB.Pool.QueryRow(ctx, `SELECT notify_prefs #>> '{types}' FROM users WHERE id=$1`, userID).Scan(&raw) != nil || raw == nil {
		return true
	}
	var types map[string]bool
	if json.Unmarshal([]byte(*raw), &types) != nil {
		return true
	}
	if v, ok := types[kind]; ok {
		return v
	}
	return true
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
