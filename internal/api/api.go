// Package api: Todorio's HTTP handlers (no public API — this is only for our own frontend,
// auth via cookie sessions).
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// Fixed set of reactions: done, blocked/no, warning, question. Narrowed from the original ten
// emoji; the client list in web/src/api.ts must stay identical, and migration 0015 removes rows
// left in the table by the wider set. The warning emoji carries the U+FE0F variation selector,
// exactly as the client sends it, so the comparison below is a plain string match.
var AllowedReactions = map[string]bool{
	"\\u2705": true, "\\u274C": true, "\\u26A0\\uFE0F": true, "\\u2753": true,
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
	// Wallpapers mirror avatars, with one difference: there is no route for reading another
	// user's. An avatar is shown next to every task and comment its owner touches; a wallpaper
	// is only ever displayed to the account it belongs to.
	mux.HandleFunc("POST /api/me/wallpaper", a.handleUploadWallpaper)
	mux.HandleFunc("DELETE /api/me/wallpaper", a.handleDeleteWallpaper)
	mux.HandleFunc("GET /api/me/wallpaper", a.handleGetWallpaper)

	// --- user administration ---
	mux.HandleFunc("GET /api/admin/users", a.handleAdminUsers)
	mux.HandleFunc("POST /api/admin/users/{id}/approve", a.handleApproveUser)
	mux.HandleFunc("POST /api/admin/users/{id}/status", a.handleSetUserStatus)
	mux.HandleFunc("POST /api/admin/users/{id}/reset-password", a.handleResetPassword)
	mux.HandleFunc("GET /api/admin/audit", a.handleAdminAudit)

	// --- spaces and lists ---
	mux.HandleFunc("POST /api/lists/{id}/duplicate", a.handleDuplicateList)
	mux.HandleFunc("POST /api/spaces/{id}/duplicate", a.handleDuplicateSpace)
	mux.HandleFunc("GET /api/spaces", a.handleListSpaces)
	mux.HandleFunc("POST /api/spaces", a.handleCreateSpace)
	mux.HandleFunc("PATCH /api/spaces/{id}", a.handleUpdateSpace)
	mux.HandleFunc("DELETE /api/spaces/{id}", a.handleArchiveSpace)
	mux.HandleFunc("POST /api/spaces/{id}/restore", a.handleRestoreSpace)
	mux.HandleFunc("DELETE /api/spaces/{id}/permanent", a.handleDeleteSpacePermanent)
	mux.HandleFunc("GET /api/spaces/{id}/archive", a.handleSpaceArchive)
	mux.HandleFunc("GET /api/archive/spaces", a.handleArchivedSpaces)
	// Member management: the grant handlers live in spaces_lists.go, the roster/role-change/
	// revoke handlers in members.go. All four are needed for the permission model to be usable
	// from the UI at all — a grant with no way to inspect or undo it isn't sharing.
	mux.HandleFunc("GET /api/spaces/{id}/members", a.handleListSpaceMembers)
	mux.HandleFunc("POST /api/spaces/{id}/members", a.handleAddSpaceMember)
	mux.HandleFunc("PATCH /api/spaces/{id}/members/{user_id}", a.handleUpdateSpaceMember)
	mux.HandleFunc("DELETE /api/spaces/{id}/members/{user_id}", a.handleRemoveSpaceMember)
	mux.HandleFunc("GET /api/spaces/{id}/lists", a.handleListLists)
	mux.HandleFunc("POST /api/spaces/{id}/lists", a.handleCreateList)
	mux.HandleFunc("PATCH /api/lists/{id}", a.handleUpdateList)
	mux.HandleFunc("DELETE /api/lists/{id}", a.handleArchiveList)
	mux.HandleFunc("POST /api/lists/{id}/restore", a.handleRestoreList)
	mux.HandleFunc("DELETE /api/lists/{id}/permanent", a.handleDeleteListPermanent)
	mux.HandleFunc("GET /api/lists/{id}/members", a.handleListListMembers)
	mux.HandleFunc("POST /api/lists/{id}/members", a.handleAddListMember)
	mux.HandleFunc("PATCH /api/lists/{id}/members/{user_id}", a.handleUpdateListMember)
	mux.HandleFunc("DELETE /api/lists/{id}/members/{user_id}", a.handleRemoveListMember)
	mux.HandleFunc("GET /api/lists/{id}/assignable", a.handleAssignableUsers)
	mux.HandleFunc("GET /api/lists/{id}/share", a.handleListShareLinks)
	mux.HandleFunc("POST /api/lists/{id}/share", a.handleCreateShareLink)
	mux.HandleFunc("DELETE /api/shares/{id}", a.handleRevokeShareLink)
	mux.HandleFunc("GET /api/public/{token}", a.handlePublicShare)
	mux.HandleFunc("POST /api/public/{token}", a.handlePublicSharePost)

	// --- tasks ---
	mux.HandleFunc("GET /api/lists/{id}/tasks", a.handleListTasks)
	mux.HandleFunc("POST /api/lists/{id}/tasks", a.handleCreateTask)
	mux.HandleFunc("GET /api/tasks/{id}", a.handleGetTask)
	mux.HandleFunc("PATCH /api/tasks/{id}", a.handleUpdateTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", a.handleArchiveTask)
	mux.HandleFunc("POST /api/tasks/{id}/restore", a.handleRestoreTask)
	mux.HandleFunc("DELETE /api/tasks/{id}/permanent", a.handleDeleteTaskPermanent)
	mux.HandleFunc("GET /api/tasks/{id}/versions", a.handleListTaskVersions)
	// --- watchers and review workflow (spec section 5) ---
	mux.HandleFunc("POST /api/tasks/{id}/watch", a.handleWatchTask)
	mux.HandleFunc("DELETE /api/tasks/{id}/watch", a.handleUnwatchTask)
	mux.HandleFunc("GET /api/tasks/{id}/watchers", a.handleListWatchers)
	mux.HandleFunc("POST /api/tasks/{id}/review/submit", a.handleSubmitReview)
	mux.HandleFunc("POST /api/tasks/{id}/review/decide", a.handleDecideReview)
	mux.HandleFunc("POST /api/tasks/{id}/versions/{version_id}/restore", a.handleRestoreTaskVersion)
	mux.HandleFunc("GET /api/my/tasks", a.handleMyTasks)
	mux.HandleFunc("GET /api/inbox", a.handleInbox)
	mux.HandleFunc("GET /api/my/stats", a.handleMyStats)
	mux.HandleFunc("GET /api/onboarding/progress", a.handleOnboardingProgress)
	mux.HandleFunc("GET /api/telegram/status", a.handleTelegramStatus)
	mux.HandleFunc("POST /api/telegram/link", a.handleTelegramLink)
	mux.HandleFunc("POST /api/telegram/unlink", a.handleTelegramUnlink)
	// Personal bot: the settings UI uses these routes to save, confirm, or remove a user's token.
	mux.HandleFunc("POST /api/me/telegram/bot", a.handleSetPersonalBot)
	mux.HandleFunc("POST /api/me/telegram/bot/confirm", a.handleConfirmPersonalBot)
	mux.HandleFunc("DELETE /api/me/telegram/bot", a.handleDeletePersonalBot)

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
	// The dashboard is the same three questions asked over a period instead of right now, so it
	// belongs next to them rather than in its own section.
	mux.HandleFunc("GET /api/spaces/{id}/dashboard", a.handleSpaceDashboard)

	// --- export / import (data portability) ---
	mux.HandleFunc("GET /api/spaces/{id}/export", a.handleExportSpace)
	mux.HandleFunc("POST /api/spaces/import", a.handleImportSpace)
	// Foreign formats are translated into the export document above and then handed to the same
	// importer, so they inherit its permission and quota checks unchanged.
	mux.HandleFunc("POST /api/import/csv", a.handleImportCSV)
	mux.HandleFunc("POST /api/import/trello", a.handleImportTrello)
	mux.HandleFunc("GET /api/spaces/{id}/workload", a.handleSpaceWorkload)

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
	mux.HandleFunc("GET /api/notes/{id}/tasks", a.handleNoteTasks)
	mux.HandleFunc("POST /api/notes/{id}/tasks", a.handleCreateTasksFromNote)

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
	mux.HandleFunc("GET /api/focus/current", a.handleCurrentFocus)
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

// maxJSONBody caps how much of a request body a JSON handler will read.
//
// json.Decoder reads until EOF. Uploads are bounded (limits.uploads.max_file_size_mb, plus
// http.MaxBytesReader on the multipart handlers), but every JSON endpoint accepted a body of any
// size: one authenticated POST streaming gigabytes of digits into a number field is enough to
// drive the process into swap or the OOM killer, with no upload quota or action counter involved.
// Since readJSON is the single entry point every handler shares, one limit here covers all of
// them at once.
//
// 1 MiB is far above anything legitimate. The largest real payloads are task descriptions and
// note bodies (text fields on the order of kilobytes); bulk imports come in through the
// attachment upload path, which has its own, larger limit.
const maxJSONBody = 1 << 20

func readJSON(r *http.Request, dst any) error {
	// LimitReader rather than http.MaxBytesReader: readJSON has no ResponseWriter to hand it,
	// and a truncated body already fails decoding, which every caller turns into a 400.
	dec := json.NewDecoder(io.LimitReader(r.Body, maxJSONBody))
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
// limit <= 0 means unlimited: nothing to do. That stays an explicit admin choice (spec section
// 10), but it is no longer the *default*. Shipping the default as 0 meant every install ran with
// no cap at all: sessions live for 30 days, so an account that signs in from a new browser now
// and then accumulates dozens of valid cookies, each one an independent way in, and a token
// stolen from a laptop that was replaced a year ago still works. Ten is generous for real use
// (phone, laptop, work machine, a few stale ones) and bounds the blast radius; an operator who
// genuinely wants unlimited can still set 0 explicitly.
func (a *API) enforceSessionLimit(ctx context.Context, userID int64) {
	limit := a.intSetting(ctx, "limits.login.max_sessions_per_user", 10)
	if limit <= 0 {
		return
	}
	// Previously a separate SELECT count(*) followed by a DELETE ... LIMIT: two parallel logins
	// could both read the count before either eviction landed, so both saw "under the limit" and
	// the account ended up with more live sessions than configured. A single statement that
	// ranks every live session by age and deletes everything past the limit removes the gap
	// between reading the count and acting on it — there is no longer a separate read to race.
	_, _ = a.DB.Pool.Exec(ctx, `
		DELETE FROM sessions WHERE id IN (
			SELECT id FROM (
				SELECT id, row_number() OVER (ORDER BY created_at DESC) AS rn
				FROM sessions WHERE user_id=$1 AND expires_at > now()
			) ranked WHERE rn > $2)`, userID, limit)
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
// whole uploads directory (task attachments, avatars, wallpapers, the instance logo — everything
// under cfg.UploadsDir) past limits.uploads.max_total_storage_mb. 0 (the default) means unlimited
// — the whole check is skipped, so upgrading never suddenly caps an existing instance.
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
//
// A user whose *global* role is "viewer" is capped at "viewer" here no matter what list_members
// says. The role existed in the schema, in the admin UI and in the API type, but nothing in the
// codebase ever read it: granting such an account "editor" on a single list (or adding it to a
// space that hands out editor by default) gave it full write access, so the read-only role was
// read-only in name only. Capping in this one function covers every list-scoped handler at once,
// because they all route their access check through here.
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
	if u.IsViewer() && permAtLeast(perm, "editor") {
		return "viewer"
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

	// Coalesce a burst about the same task. Editing a task's status, then its deadline, then its
	// assignee within a minute used to produce three separate bell entries; the recipient only
	// needs to know the task changed. An unread notification of the same kind about the same task
	// inside the collapse window is refreshed in place instead of adding a new row.
	//
	// Only unread ones are merged: something already seen must not be silently rewritten, or the
	// history of what happened becomes unreliable. task_id is compared out of the JSON payload,
	// so kinds without a task (e.g. "approved") never collapse.
	if window := a.intSetting(r.Context(), "limits.notify.collapse_seconds", 120); window > 0 {
		if taskID, ok := payload["task_id"]; ok {
			var existing int64
			err := a.DB.Pool.QueryRow(r.Context(), `
				SELECT id FROM notifications
				WHERE user_id=$1 AND kind=$2 AND read_at IS NULL
				  AND payload->>'task_id' = $3
				  AND created_at > now() - make_interval(secs => $4)
				ORDER BY created_at DESC LIMIT 1`,
				userID, kind, fmt.Sprint(taskID), window).Scan(&existing)
			if err == nil {
				_, _ = a.DB.Pool.Exec(r.Context(),
					`UPDATE notifications SET payload=$2, created_at=now() WHERE id=$1`, existing, string(b))
				if a.inDoNotDisturb(r.Context(), userID) {
					return
				}
				a.Bus.Publish([]int64{userID}, events.Event{Type: "notification", Data: map[string]any{"kind": kind, "payload": payload}})
				return
			}
		}
	}

	_, _ = a.DB.Pool.Exec(r.Context(),
		`INSERT INTO notifications(user_id, kind, payload) VALUES($1,$2,$3)`, userID, kind, string(b))
	if a.inDoNotDisturb(r.Context(), userID) {
		return
	}
	a.Bus.Publish([]int64{userID}, events.Event{Type: "notification", Data: map[string]any{"kind": kind, "payload": payload}})
	// Telegram (if configured and linked) only on a genuinely new notification, not on every
	// collapse-refresh above — a burst of edits to one task should ping a phone once, not once
	// per edit, even though the in-app bell/SSE already re-fires on each refresh.
	a.sendTelegram(r.Context(), userID, kind, payload)
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
