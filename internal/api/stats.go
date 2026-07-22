package api

import (
	"net/http"
)

// GET /api/spaces/{id}/stats?period=week|month — space statistics:
// per-member (done, weighted contribution), "top performer", and a random two-part caption.
// Rankings can be disabled: spaces.settings -> stats.show_best = false.
func (a *API) handleStats(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "no access to the space")
		return
	}
	interval := "7 days"
	if r.URL.Query().Get("period") == "month" {
		interval = "30 days"
	}

	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT u.id, u.username, COALESCE(u.display_name, u.username),
			count(t.id) FILTER (WHERE t.completed_at > now() - $2::interval)::int AS done_cnt,
			COALESCE(sum(t.weight) FILTER (WHERE t.completed_at > now() - $2::interval), 0)::int AS done_weight,
			count(t.id) FILTER (WHERE t.completed_at IS NULL AND t.due_at < now())::int AS overdue_cnt
		FROM space_members m
		JOIN users u ON u.id = m.user_id AND u.status = 'active'
		LEFT JOIN tasks t ON t.assignee_id = u.id AND t.archived_at IS NULL
			AND t.list_id IN (SELECT id FROM lists WHERE space_id = $1 AND archived_at IS NULL)
		WHERE m.space_id = $1
		GROUP BY u.id, u.username, u.display_name
		ORDER BY done_weight DESC, done_cnt DESC`, spaceID, interval)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	type member struct {
		ID         int64  `json:"id"`
		Username   string `json:"username"`
		Name       string `json:"name"`
		Done       int    `json:"done"`
		DoneWeight int    `json:"done_weight"`
		Overdue    int    `json:"overdue"`
	}
	members := []member{}
	totalDone, totalOverdue := 0, 0
	for rows.Next() {
		var m member
		if rows.Scan(&m.ID, &m.Username, &m.Name, &m.Done, &m.DoneWeight, &m.Overdue) == nil {
			members = append(members, m)
			totalDone += m.Done
			totalOverdue += m.Overdue
		}
	}

	// Caption category based on the period's state.
	category := "neutral"
	switch {
	case totalOverdue > totalDone && totalOverdue > 0:
		category = "overdue"
	case totalDone >= 10:
		category = "success"
	case totalDone >= 3:
		category = "focus"
	}

	// Two-part caption: deterministic "random" pick per day (seed = space_id + day of year),
	// so the caption doesn't change on every page refresh.
	locale := a.DB.Setting(r.Context(), "branding.stats_locale", "en-US")
	var part1, part2 string
	_ = a.DB.Pool.QueryRow(r.Context(), `
		SELECT text FROM stat_captions WHERE locale=$1 AND category=$2 AND part=1
		ORDER BY id OFFSET (($3 + EXTRACT(DOY FROM now())::int) % GREATEST(
			(SELECT count(*) FROM stat_captions WHERE locale=$1 AND category=$2 AND part=1), 1)) LIMIT 1`,
		locale, category, spaceID).Scan(&part1)
	_ = a.DB.Pool.QueryRow(r.Context(), `
		SELECT text FROM stat_captions WHERE locale=$1 AND category=$2 AND part=2
		ORDER BY id OFFSET (($3 * 7 + EXTRACT(DOY FROM now())::int * 3) % GREATEST(
			(SELECT count(*) FROM stat_captions WHERE locale=$1 AND category=$2 AND part=2), 1)) LIMIT 1`,
		locale, category, spaceID).Scan(&part2)

	resp := map[string]any{
		"period":   interval,
		"members":  members,
		"caption":  map[string]string{"part1": part1, "part2": part2, "category": category},
	}

	// "Top performer" — configurable by the space owner (settings.stats.show_best).
	var showBest *bool
	_ = a.DB.Pool.QueryRow(r.Context(),
		`SELECT (settings #>> '{stats,show_best}')::boolean FROM spaces WHERE id=$1`, spaceID).Scan(&showBest)
	if (showBest == nil || *showBest) && len(members) > 0 && members[0].DoneWeight > 0 {
		resp["best"] = members[0]
	}

	writeJSON(w, http.StatusOK, resp)
}
