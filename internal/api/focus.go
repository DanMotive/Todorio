package api

// Focus mode: a simple time-tracking session, optionally tied to one task. Only one open session
// per user at a time — starting a new one closes the previous.
//
// Task-linked sessions double as a lightweight presence signal (spec: show who is currently
// working on a task, with a timestamp). Starting/stopping a session on a task broadcasts
// focus.started / focus.stopped over the existing SSE bus to everyone with access to that task's
// list, and the open session itself is surfaced on every task read (see taskSelect's
// active_focus subquery in tasks.go) so a freshly loaded board is correct even without SSE.

import (
	"net/http"

	"github.com/DanMotive/Todorio/internal/auth"
	"github.com/DanMotive/Todorio/internal/events"
)

// POST /api/focus/start {task_id?}
func (a *API) handleStartFocus(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var in struct {
		TaskID *int64 `json:"task_id"`
	}
	_ = readJSON(r, &in)

	if prev := a.endOpenFocusSession(r, u.ID); prev != nil {
		a.announceFocus(r, *prev, "focus.stopped", u)
	}

	var id int64
	err := a.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO focus_sessions(user_id, task_id) VALUES($1,$2) RETURNING id`, u.ID, in.TaskID).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	if in.TaskID != nil {
		a.announceFocus(r, *in.TaskID, "focus.started", u)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// POST /api/focus/stop
func (a *API) handleStopFocus(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	prev := a.endOpenFocusSession(r, u.ID)
	if prev != nil {
		a.announceFocus(r, *prev, "focus.stopped", u)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// endOpenFocusSession closes the caller's open session, if any, and returns the task_id it was
// tied to (nil if there was none open, or it wasn't tied to a task) — used both to report "stop"
// results and to know who to notify. Zero matching rows is a normal, silent no-op, not an error.
func (a *API) endOpenFocusSession(r *http.Request, userID int64) *int64 {
	rows, err := a.DB.Pool.Query(r.Context(), `
		UPDATE focus_sessions SET ended_at=now(), duration_seconds=EXTRACT(EPOCH FROM (now()-started_at))::int
		WHERE user_id=$1 AND ended_at IS NULL RETURNING task_id`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var taskID *int64
	if rows.Next() {
		_ = rows.Scan(&taskID)
	}
	return taskID
}

// announceFocus resolves a task's list and broadcasts a presence event to everyone with access to
// it, so an open board updates live instead of only on next reload.
func (a *API) announceFocus(r *http.Request, taskID int64, eventType string, u *auth.User) {
	var listID int64
	if a.DB.Pool.QueryRow(r.Context(), `SELECT list_id FROM tasks WHERE id=$1`, taskID).Scan(&listID) != nil {
		return
	}
	a.publishToListMembers(r, listID, events.Event{
		Type: eventType,
		Data: map[string]any{"task_id": taskID, "user_id": u.ID, "username": u.Username},
	})
}

// GET /api/focus/stats?period=week|month — total focused time (for the profile/stats screen).
func (a *API) handleFocusStats(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	interval := "7 days"
	if r.URL.Query().Get("period") == "month" {
		interval = "30 days"
	}
	var totalSeconds int
	var sessionCount int
	_ = a.DB.Pool.QueryRow(r.Context(), `
		SELECT COALESCE(SUM(duration_seconds),0), count(*)
		FROM focus_sessions
		WHERE user_id=$1 AND started_at > now() - $2::interval AND ended_at IS NOT NULL`,
		u.ID, interval).Scan(&totalSeconds, &sessionCount)
	writeJSON(w, http.StatusOK, map[string]any{"total_seconds": totalSeconds, "sessions": sessionCount})
}
