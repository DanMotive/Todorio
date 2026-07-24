// Personal statistics (spec section 14: "за июль: закрыто 47 · вовремя 79% · просрочено 6 ·
// самый активный список · время в фокусе") — distinct from handleStats (per-space leaderboard).
// This aggregates across every space/list the user has tasks in, not just one space.
package api

import "net/http"

// GET /api/my/stats?period=week|month
func (a *API) handleMyStats(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if !a.featureEnabled(r.Context(), "stats") {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	interval := "7 days"
	if r.URL.Query().Get("period") == "month" {
		interval = "30 days"
	}

	var done, onTime, overdue int
	_ = a.DB.Pool.QueryRow(r.Context(), `
		SELECT
			count(*) FILTER (WHERE completed_at > now() - $2::interval)::int,
			count(*) FILTER (WHERE completed_at > now() - $2::interval AND (due_at IS NULL OR completed_at <= due_at))::int,
			count(*) FILTER (WHERE completed_at IS NULL AND due_at < now())::int
		FROM tasks WHERE assignee_id=$1 AND archived_at IS NULL`,
		u.ID, interval).Scan(&done, &onTime, &overdue)
	onTimePct := 0
	if done > 0 {
		onTimePct = onTime * 100 / done
	}

	var mostActiveList *string
	var mostActiveCount int
	_ = a.DB.Pool.QueryRow(r.Context(), `
		SELECT l.name, count(*)::int FROM tasks t JOIN lists l ON l.id = t.list_id
		WHERE t.assignee_id=$1 AND t.archived_at IS NULL AND t.completed_at > now() - $2::interval
		GROUP BY l.name ORDER BY count(*) DESC LIMIT 1`,
		u.ID, interval).Scan(&mostActiveList, &mostActiveCount)

	var focusSeconds, focusSessions int
	_ = a.DB.Pool.QueryRow(r.Context(), `
		SELECT COALESCE(SUM(duration_seconds),0), count(*)
		FROM focus_sessions
		WHERE user_id=$1 AND started_at > now() - $2::interval AND ended_at IS NOT NULL`,
		u.ID, interval).Scan(&focusSeconds, &focusSessions)

	writeJSON(w, http.StatusOK, map[string]any{
		"period": interval, "done": done, "on_time_pct": onTimePct, "overdue": overdue,
		"most_active_list": mostActiveList, "most_active_list_count": mostActiveCount,
		"focus_seconds": focusSeconds, "focus_sessions": focusSessions,
	})
}
