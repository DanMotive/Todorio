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
	"time"

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

// closeFocusForTask ends every open focus session tied to a task, for every user, and announces
// it. Called when a task is completed or archived: continuing to bill time against work that is
// finished is simply wrong, and the timer would keep ticking in the sidebar with nothing left to
// track. Sessions are closed for all users, not just the caller — if two people were focused on
// the task, both of their timers have to stop.
//
// The elapsed time already worked is preserved (duration_seconds is computed on close), so the
// user still gets credit for it in their stats.
func (a *API) closeFocusForTask(r *http.Request, taskID int64, u *auth.User) {
	rows, err := a.DB.Pool.Query(r.Context(), `
		UPDATE focus_sessions
		SET ended_at = now(), duration_seconds = EXTRACT(EPOCH FROM (now() - started_at))::int
		WHERE task_id = $1 AND ended_at IS NULL
		RETURNING user_id`, taskID)
	if err != nil {
		dbFail(r, "close focus for task", err)
		return
	}
	var closed int
	for rows.Next() {
		var uid int64
		if rows.Scan(&uid) == nil {
			closed++
		}
	}
	rows.Close()
	if closed > 0 {
		a.announceFocus(r, taskID, "focus.stopped", u)
	}
}

// closeFocusForTaskTree is closeFocusForTask extended to a task and its subtasks, matching the
// archive cascade (a task is archived together with its children).
func (a *API) closeFocusForTaskTree(r *http.Request, taskID int64, u *auth.User) {
	rows, err := a.DB.Pool.Query(r.Context(), `
		UPDATE focus_sessions
		SET ended_at = now(), duration_seconds = EXTRACT(EPOCH FROM (now() - started_at))::int
		WHERE ended_at IS NULL
		  AND task_id IN (SELECT id FROM tasks WHERE id = $1 OR parent_id = $1)
		RETURNING task_id`, taskID)
	if err != nil {
		dbFail(r, "close focus for task tree", err)
		return
	}
	seen := map[int64]bool{}
	for rows.Next() {
		var tid *int64
		if rows.Scan(&tid) == nil && tid != nil {
			seen[*tid] = true
		}
	}
	rows.Close()
	for tid := range seen {
		a.announceFocus(r, tid, "focus.stopped", u)
	}
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

// GET /api/focus/current — the caller's open focus session, if any.
//
// Needed because the timer UI used to keep its state purely in the component: closing the task
// modal unmounted it and the running timer appeared to reset, even though the server session was
// still open. The elapsed time is derived from started_at here, so the clock survives navigation,
// a page reload, and even a different device.
func (a *API) handleCurrentFocus(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var (
		id        int64
		taskID    *int64
		taskTitle *string
		startedAt time.Time
	)
	err := a.DB.Pool.QueryRow(r.Context(), `
		SELECT fs.id, fs.task_id, t.title, fs.started_at
		FROM focus_sessions fs
		LEFT JOIN tasks t ON t.id = fs.task_id
		WHERE fs.user_id = $1 AND fs.ended_at IS NULL
		ORDER BY fs.started_at DESC LIMIT 1`, u.ID).Scan(&id, &taskID, &taskTitle, &startedAt)
	if err != nil {
		// No open session is the normal case, not an error.
		writeJSON(w, http.StatusOK, map[string]any{"running": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"running":    true,
		"id":         id,
		"task_id":    taskID,
		"task_title": taskTitle,
		"started_at": startedAt,
		// Server-computed so a client with a skewed clock still shows the right elapsed time.
		"elapsed_seconds": int(time.Since(startedAt).Seconds()),
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
