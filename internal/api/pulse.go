package api

import "net/http"

// GET /api/spaces/{id}/pulse — "Space Pulse": a live health summary.
// Signals: overdue, unassigned, no deadline, blocked, stalled (>3 days with no movement).
func (a *API) handlePulse(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "no access to the space")
		return
	}
	var (
		total, open, done, overdue, unassigned, noDeadline, blocked, stale int
	)
	err = a.DB.Pool.QueryRow(r.Context(), `
		SELECT
			count(*)::int,
			count(*) FILTER (WHERE t.completed_at IS NULL)::int,
			count(*) FILTER (WHERE t.completed_at IS NOT NULL)::int,
			count(*) FILTER (WHERE t.completed_at IS NULL AND t.due_at < now())::int,
			count(*) FILTER (WHERE t.completed_at IS NULL AND t.assignee_id IS NULL)::int,
			count(*) FILTER (WHERE t.completed_at IS NULL AND t.due_at IS NULL)::int,
			count(*) FILTER (WHERE t.completed_at IS NULL AND COALESCE(array_length(t.blocked_by,1),0) > 0)::int,
			count(*) FILTER (WHERE t.completed_at IS NULL AND t.updated_at < now() - interval '3 days')::int
		FROM tasks t JOIN lists l ON l.id = t.list_id
		WHERE l.space_id=$1 AND t.archived_at IS NULL AND l.archived_at IS NULL`,
		spaceID).Scan(&total, &open, &done, &overdue, &unassigned, &noDeadline, &blocked, &stale)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}

	// Health score 0..100: penalties for overdue/stalled/blocked items.
	score := 100
	if open > 0 {
		score -= min(50, overdue*100/open/2)
		score -= min(25, stale*100/open/4)
		score -= min(15, blocked*100/open/4)
		score -= min(10, unassigned*100/open/10)
	}
	if score < 0 {
		score = 0
	}
	mood := "\U0001F7E2" // green
	if score < 70 {
		mood = "\U0001F7E1"
	}
	if score < 40 {
		mood = "\U0001F534"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"score": score, "mood": mood,
		"total": total, "open": open, "done": done,
		"signals": map[string]int{
			"overdue": overdue, "unassigned": unassigned, "no_deadline": noDeadline,
			"blocked": blocked, "stale": stale,
		},
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
