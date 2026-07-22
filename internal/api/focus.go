package api

// Focus mode: a simple time-tracking session, optionally tied to one task.
// Only one open session per user at a time — starting a new one closes the previous.

import "net/http"

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
	_, _ = a.DB.Pool.Exec(r.Context(), `
		UPDATE focus_sessions SET ended_at=now(), duration_seconds=EXTRACT(EPOCH FROM (now()-started_at))::int
		WHERE user_id=$1 AND ended_at IS NULL`, u.ID)
	var id int64
	err := a.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO focus_sessions(user_id, task_id) VALUES($1,$2) RETURNING id`, u.ID, in.TaskID).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// POST /api/focus/stop
func (a *API) handleStopFocus(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	_, err := a.DB.Pool.Exec(r.Context(), `
		UPDATE focus_sessions SET ended_at=now(), duration_seconds=EXTRACT(EPOCH FROM (now()-started_at))::int
		WHERE user_id=$1 AND ended_at IS NULL`, u.ID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
