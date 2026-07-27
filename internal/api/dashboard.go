package api

// Space dashboard.
//
// The space already answers narrow questions well: pulse says what is wrong right now, workload
// says who is carrying what, stats says what was finished. What none of them answers is the one
// a person actually opens a space to ask - "how is this space doing, compared with a week ago".
// Reading that off three separate screens means holding three sets of numbers in your head and
// doing the comparison yourself.
//
// This endpoint returns that comparison already made: the same rows every other space query
// reads, aggregated over a period, in one response. No new tables, no new columns, and no write
// path - it is strictly a different way of looking at what is already recorded.

import (
	"net/http"
)

// dashboardPeriods maps the two periods the UI offers onto day counts. A closed vocabulary
// rather than a free ?days= number: every widget below (the daily series in particular) is laid
// out for a week or a month, and an arbitrary 273-day request would render as an unreadable
// smear of one-pixel columns while costing the database real work.
var dashboardPeriods = map[string]int{"week": 7, "month": 30}

const dashboardDefaultPeriod = "week"

// dashboardTopOverdue caps the "most overdue" list. Five is a list someone will actually read
// and act on; a full backlog dump belongs in the task list, which already has filters for it.
const dashboardTopOverdue = 5

type dashboardSummary struct {
	OpenCount    int `json:"open_count"`
	OverdueCount int `json:"overdue_count"`
	// DoneInPeriod counts tasks closed inside the window, not tasks that happen to be closed
	// now - the difference is the whole point of a period view.
	DoneInPeriod int `json:"done_in_period"`
	// AvgCloseHours is the mean created -> completed time of those same tasks. Reported in
	// hours and left unrounded: the client decides how to phrase it, and rounding here would
	// throw away the distinction between "under an hour" and "a few minutes".
	AvgCloseHours float64 `json:"avg_close_hours"`
	OpenWeight    int     `json:"open_weight"`
}

type dashboardStatusRow struct {
	// Status is whatever the space's workflow defines, not a fixed enum: a space with custom
	// statuses must see its own vocabulary here, so this is grouped by the stored value and
	// the client labels what it recognises.
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type dashboardPersonRow struct {
	// UserID is nil for the unassigned bucket. Work nobody has picked up is part of how a
	// space is doing, so it is reported alongside the people rather than dropped.
	UserID       *int64  `json:"user_id"`
	Username     *string `json:"username"`
	Name         *string `json:"name"`
	OpenCount    int     `json:"open_count"`
	OverdueCount int     `json:"overdue_count"`
}

type dashboardDay struct {
	Day     string `json:"day"`
	Created int    `json:"created"`
	Done    int    `json:"done"`
}

type dashboardOverdueTask struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	ListID   int64   `json:"list_id"`
	ListName string  `json:"list_name"`
	DueAt    string  `json:"due_at"`
	DaysLate int     `json:"days_late"`
	Assignee *string `json:"assignee"`
}

// GET /api/spaces/{id}/dashboard?period=week|month
//
// Any space member may look, including a read-only one: this is the same information their task
// lists already show them, only added up. A dashboard the team cannot see cannot do the job a
// dashboard exists for, which is letting a team notice its own trend without being told.
func (a *API) handleSpaceDashboard(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "no access to the space")
		return
	}

	period := r.URL.Query().Get("period")
	days, ok := dashboardPeriods[period]
	if !ok {
		period = dashboardDefaultPeriod
		days = dashboardPeriods[dashboardDefaultPeriod]
	}

	// Private lists stay private. This predicate is repeated in every query below rather than
	// factored into a view, so that each one can be read - and audited - on its own; an
	// aggregate that quietly widened visibility would be much harder to notice than a missing
	// row. Admins see the whole space, matching every other admin-facing view.
	const visible = `
		  AND t.archived_at IS NULL
		  AND l.archived_at IS NULL
		  AND l.space_id = $1
		  AND ($2 OR l.is_private = false OR l.id IN (
				SELECT list_id FROM list_members WHERE user_id = $3
		  ))`

	var sum dashboardSummary
	err = a.DB.Pool.QueryRow(r.Context(), `
		SELECT
			count(*) FILTER (WHERE t.completed_at IS NULL)::int,
			count(*) FILTER (WHERE t.completed_at IS NULL AND t.due_at IS NOT NULL AND t.due_at < now())::int,
			count(*) FILTER (WHERE t.completed_at >= now() - make_interval(days => $4))::int,
			COALESCE(avg(EXTRACT(EPOCH FROM (t.completed_at - t.created_at)) / 3600.0)
				FILTER (WHERE t.completed_at >= now() - make_interval(days => $4)), 0)::float8,
			COALESCE(sum(t.weight) FILTER (WHERE t.completed_at IS NULL), 0)::int
		FROM tasks t
		JOIN lists l ON l.id = t.list_id
		WHERE true`+visible,
		spaceID, u.IsAdmin(), u.ID, days).Scan(
		&sum.OpenCount, &sum.OverdueCount, &sum.DoneInPeriod, &sum.AvgCloseHours, &sum.OpenWeight)
	if err != nil {
		dbFail(r, "space dashboard summary", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}

	statuses := []dashboardStatusRow{}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT t.status, count(*)::int
		FROM tasks t
		JOIN lists l ON l.id = t.list_id
		WHERE t.completed_at IS NULL`+visible+`
		GROUP BY t.status
		ORDER BY 2 DESC`,
		spaceID, u.IsAdmin(), u.ID)
	if err != nil {
		dbFail(r, "space dashboard statuses", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	for rows.Next() {
		var sr dashboardStatusRow
		if rows.Scan(&sr.Status, &sr.Count) == nil {
			statuses = append(statuses, sr)
		}
	}
	rows.Close()

	people := []dashboardPersonRow{}
	rows, err = a.DB.Pool.Query(r.Context(), `
		SELECT us.id, us.username, us.name,
			count(*)::int,
			count(*) FILTER (WHERE t.due_at IS NOT NULL AND t.due_at < now())::int
		FROM tasks t
		JOIN lists l ON l.id = t.list_id
		LEFT JOIN users us ON us.id = t.assignee_id
		WHERE t.completed_at IS NULL`+visible+`
		GROUP BY us.id, us.username, us.name
		ORDER BY 4 DESC`,
		spaceID, u.IsAdmin(), u.ID)
	if err != nil {
		dbFail(r, "space dashboard people", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	for rows.Next() {
		var pr dashboardPersonRow
		if rows.Scan(&pr.UserID, &pr.Username, &pr.Name, &pr.OpenCount, &pr.OverdueCount) == nil {
			people = append(people, pr)
		}
	}
	rows.Close()

	// One row per calendar day, including the days on which nothing happened: a series with the
	// empty days dropped would draw a chart that silently compresses a quiet week into a busy
	// one. generate_series supplies the spine, the two counts hang off it.
	series := []dashboardDay{}
	rows, err = a.DB.Pool.Query(r.Context(), `
		SELECT to_char(d, 'YYYY-MM-DD'),
			(SELECT count(*)::int FROM tasks t JOIN lists l ON l.id = t.list_id
			 WHERE t.created_at::date = d::date`+visible+`),
			(SELECT count(*)::int FROM tasks t JOIN lists l ON l.id = t.list_id
			 WHERE t.completed_at::date = d::date`+visible+`)
		FROM generate_series(current_date - make_interval(days => $4 - 1), current_date, interval '1 day') AS d
		ORDER BY d`,
		spaceID, u.IsAdmin(), u.ID, days)
	if err != nil {
		dbFail(r, "space dashboard series", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	for rows.Next() {
		var dd dashboardDay
		if rows.Scan(&dd.Day, &dd.Created, &dd.Done) == nil {
			series = append(series, dd)
		}
	}
	rows.Close()

	overdue := []dashboardOverdueTask{}
	rows, err = a.DB.Pool.Query(r.Context(), `
		SELECT t.id, t.title, l.id, l.name, to_char(t.due_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
			EXTRACT(DAY FROM (now() - t.due_at))::int,
			COALESCE(us.name, us.username)
		FROM tasks t
		JOIN lists l ON l.id = t.list_id
		LEFT JOIN users us ON us.id = t.assignee_id
		WHERE t.completed_at IS NULL AND t.due_at IS NOT NULL AND t.due_at < now()`+visible+`
		ORDER BY t.due_at
		LIMIT $4`,
		spaceID, u.IsAdmin(), u.ID, dashboardTopOverdue)
	if err != nil {
		dbFail(r, "space dashboard overdue", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	for rows.Next() {
		var ot dashboardOverdueTask
		if rows.Scan(&ot.ID, &ot.Title, &ot.ListID, &ot.ListName, &ot.DueAt, &ot.DaysLate, &ot.Assignee) == nil {
			overdue = append(overdue, ot)
		}
	}
	rows.Close()

	writeJSON(w, http.StatusOK, map[string]any{
		"period":       period,
		"days":         days,
		"summary":      sum,
		"by_status":    statuses,
		"by_assignee":  people,
		"series":       series,
		"top_overdue":  overdue,
	})
}
