package api

// Team workload.
//
// tasks.weight has been in the schema since 0001 and is shown in the stats table, but only ever
// backwards: how much was finished. Nobody could see what is still coming. "Who is buried and
// who is free" is the question a space owner actually asks before handing out the next task, and
// until now the only way to answer it was to open every list and count by eye.
//
// This reports the same weight numbers looking forward instead: what is open, what is already
// late, and what falls inside the next N days, grouped by assignee. No new columns - the data
// was always there.

import (
	"net/http"
	"strconv"
)

// workloadDefaultDays is the planning horizon when the caller doesn't specify one. A week is the
// span most teams actually plan over, and it matches the existing stats period vocabulary.
const workloadDefaultDays = 7

// workloadMaxDays caps the horizon. Beyond a quarter the "due soon" bucket stops meaning
// anything - it just becomes the open bucket again.
const workloadMaxDays = 90

type workloadRow struct {
	// UserID is nil for the unassigned bucket, which is reported alongside the people: work
	// nobody has picked up is exactly what a capacity view needs to surface.
	UserID   *int64  `json:"user_id"`
	Username *string `json:"username"`
	Name     *string `json:"name"`

	OpenCount  int `json:"open_count"`
	OpenWeight int `json:"open_weight"`

	OverdueCount  int `json:"overdue_count"`
	OverdueWeight int `json:"overdue_weight"`

	// DueSoon counts work with a deadline inside the horizon. Undated tasks are deliberately
	// left out of this bucket (they are still in Open): a task with no deadline is not "due
	// this week", and quietly folding it in would overstate the load.
	DueSoonCount  int `json:"due_soon_count"`
	DueSoonWeight int `json:"due_soon_weight"`

	// Blocked is open work that cannot be started yet. It counts towards a person's queue but
	// not towards what they can do today, which is a different thing entirely - showing them
	// merged would make someone look busy when they are actually stuck.
	BlockedCount int `json:"blocked_count"`
}

// GET /api/spaces/{id}/workload?days=7
//
// Any space member may look. This is the same information the space's task lists already expose
// to them, only summed up - and a capacity view that only the owner can see cannot serve its
// main purpose, which is letting a team notice its own imbalance without being told.
func (a *API) handleSpaceWorkload(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "no access to the space")
		return
	}

	days := workloadDefaultDays
	if raw := r.URL.Query().Get("days"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			days = n
		}
	}
	if days > workloadMaxDays {
		days = workloadMaxDays
	}

	// Private lists stay private: the same visibility rule the task and search queries use, so a
	// summary can never become a side channel for work the caller cannot open. Admins see the
	// whole space, matching every other admin-facing view.
	//
	// The display name column is users.display_name - there is no users.name, and asking for one
	// made Postgres reject the whole statement.
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT
			u.id, u.username, u.display_name,
			count(*)::int,
			COALESCE(sum(t.weight), 0)::int,
			count(*) FILTER (WHERE t.due_at IS NOT NULL AND t.due_at < now())::int,
			COALESCE(sum(t.weight) FILTER (WHERE t.due_at IS NOT NULL AND t.due_at < now()), 0)::int,
			count(*) FILTER (WHERE t.due_at >= now() AND t.due_at < now() + make_interval(days => $4))::int,
			COALESCE(sum(t.weight) FILTER (WHERE t.due_at >= now() AND t.due_at < now() + make_interval(days => $4)), 0)::int,
			count(*) FILTER (WHERE EXISTS (
				SELECT 1 FROM tasks b
				WHERE b.id = ANY(t.blocked_by) AND b.completed_at IS NULL AND b.archived_at IS NULL
			))::int
		FROM tasks t
		JOIN lists l ON l.id = t.list_id
		LEFT JOIN users u ON u.id = t.assignee_id
		WHERE l.space_id = $1
		  AND t.archived_at IS NULL
		  AND l.archived_at IS NULL
		  AND t.completed_at IS NULL
		  AND ($2 OR l.is_private = false OR l.id IN (
				SELECT list_id FROM list_members WHERE user_id = $3
		  ))
		GROUP BY u.id, u.username, u.display_name
		ORDER BY 5 DESC, 4 DESC`,
		spaceID, u.IsAdmin(), u.ID, days)
	if err != nil {
		dbFail(r, "space workload", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	members := []workloadRow{}
	var unassigned *workloadRow
	totals := workloadRow{}
	for rows.Next() {
		var wr workloadRow
		if rows.Scan(&wr.UserID, &wr.Username, &wr.Name,
			&wr.OpenCount, &wr.OpenWeight,
			&wr.OverdueCount, &wr.OverdueWeight,
			&wr.DueSoonCount, &wr.DueSoonWeight,
			&wr.BlockedCount) != nil {
			continue
		}
		totals.OpenCount += wr.OpenCount
		totals.OpenWeight += wr.OpenWeight
		totals.OverdueCount += wr.OverdueCount
		totals.OverdueWeight += wr.OverdueWeight
		totals.DueSoonCount += wr.DueSoonCount
		totals.DueSoonWeight += wr.DueSoonWeight
		totals.BlockedCount += wr.BlockedCount
		if wr.UserID == nil {
			copy := wr
			unassigned = &copy
			continue
		}
		members = append(members, wr)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"days":       days,
		"members":    members,
		"unassigned": unassigned,
		"totals":     totals,
		// The fair share one person would carry if the remaining weight were split evenly. It is
		// a reference line for the UI, not a target: the point is to make an imbalance visible,
		// not to imply everyone should be at the same number.
		"even_share": evenShare(totals.OpenWeight, len(members)),
	})
}

// evenShare divides total weight across the people who hold any of it. Returns 0 when nobody
// does, which the UI reads as "no reference line to draw".
func evenShare(totalWeight, people int) int {
	if people <= 0 {
		return 0
	}
	return totalWeight / people
}
