package api

// Timeline / Gantt view (spec section 12: "представления: Список, Kanban, Календарь,
// Таймлайн, Таблица, «Моя неделя»").
//
// A Gantt bar needs a start and an end. tasks.start_at (migration 0007) supplies the start
// when the user set one, but most tasks only ever get a deadline — so this endpoint derives a
// usable range per task rather than dropping those from the chart:
//
//	start_at + due_at   -> exactly that range (the explicit case)
//	due_at only         -> a one-day bar ending on the deadline; "implied": true
//	start_at only       -> from the start to the window's end, i.e. open-ended; "implied": true
//	neither             -> excluded, and reported separately in "unscheduled"
//
// Marking derived ranges as implied matters: the frontend renders them differently (hatched,
// lower contrast) so a bar the user actually scheduled is never confused with one the server
// guessed. Inventing precise-looking dates and presenting them as fact would be worse than
// showing nothing.
//
// Dependencies (tasks.blocked_by) are returned as links so the chart can draw arrows, but only
// links whose *both* ends are inside the returned window — an arrow to an off-screen bar has
// nowhere to point.

import (
	"net/http"
	"strconv"
	"time"
)

type timelineItem struct {
	ID          int64      `json:"id"`
	ListID      int64      `json:"list_id"`
	ListName    string     `json:"list_name"`
	ParentID    *int64     `json:"parent_id"`
	Title       string     `json:"title"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	Assignee    *string    `json:"assignee"`
	Start       time.Time  `json:"start"`
	End         time.Time  `json:"end"`
	Implied     bool       `json:"implied"`
	Progress    int        `json:"progress"`
	Overdue     bool       `json:"overdue"`
	Done        bool       `json:"done"`
	BlockedBy   []int64    `json:"blocked_by"`
	CompletedAt *time.Time `json:"completed_at"`
	// CanEdit mirrors listPermission(...) >= editor, computed from the list_members row already
	// joined for the visibility check below — no extra query per row. The frontend uses it to
	// decide whether a bar can be dragged/resized; handleUpdateTask re-checks the same
	// permission server-side regardless, so this is purely a UI affordance, not the security
	// boundary.
	CanEdit bool `json:"can_edit"`
}

type timelineLink struct {
	From int64 `json:"from"` // the blocking task
	To   int64 `json:"to"`   // the task that is blocked
}

// GET /api/spaces/{id}/timeline?from=YYYY-MM-DD&to=YYYY-MM-DD[&list_id=N]
//
// Scoped to a space (optionally narrowed to one list) because a Gantt chart is only useful
// across a project, not inside a single list.
func (a *API) handleTimeline(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "no access to the space")
		return
	}

	// Default window: the current month plus the next two — wide enough to be useful on first
	// open, narrow enough not to pull a project's entire history.
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	to := from.AddDate(0, 3, 0)
	if q := r.URL.Query().Get("from"); q != "" {
		if d, err := time.Parse("2006-01-02", q); err == nil {
			from = d
		}
	}
	if q := r.URL.Query().Get("to"); q != "" {
		if d, err := time.Parse("2006-01-02", q); err == nil {
			to = d
		}
	}
	if !to.After(from) {
		errJSON(w, http.StatusBadRequest, "'to' must be after 'from'")
		return
	}
	// Cap the window so a hand-crafted request can't ask for a decade and force the server to
	// serialise every task in the space.
	if to.Sub(from) > 366*24*time.Hour {
		to = from.AddDate(1, 0, 0)
	}

	// Optional single-list filter. 0 means "the whole space".
	var listFilter int64
	if q := r.URL.Query().Get("list_id"); q != "" {
		if n, err := strconv.ParseInt(q, 10, 64); err == nil {
			listFilter = n
		}
	}

	// Only lists the caller can actually see: private lists they're not a member of must not
	// leak into the chart. Mirrors the visibility rule in handleListLists.
	isAdmin := u.IsAdmin()
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT t.id, t.list_id, l.name, t.parent_id, t.title, t.status, t.priority,
			usr.username, t.start_at, t.due_at, t.progress,
			(SELECT count(*) FROM tasks s WHERE s.parent_id=t.id AND s.archived_at IS NULL AND s.completed_at IS NOT NULL)::int,
			(SELECT count(*) FROM tasks s WHERE s.parent_id=t.id AND s.archived_at IS NULL)::int,
			COALESCE(t.blocked_by, '{}'), t.completed_at, lm.permission
		FROM tasks t
		JOIN lists l ON l.id = t.list_id
		LEFT JOIN list_members lm ON lm.list_id = l.id AND lm.user_id = $2
		LEFT JOIN users usr ON usr.id = t.assignee_id
		WHERE l.space_id = $1
		  AND t.archived_at IS NULL AND l.archived_at IS NULL
		  AND ($3 OR lm.user_id IS NOT NULL OR l.is_private = false)
		  AND ($4 = 0 OR t.list_id = $4)
		  AND (t.start_at IS NOT NULL OR t.due_at IS NOT NULL)
		  -- overlap test: the task's own range must intersect the requested window
		  AND COALESCE(t.start_at, t.due_at) < $6
		  AND COALESCE(t.due_at, t.start_at) >= $5
		ORDER BY COALESCE(t.start_at, t.due_at), t.id`,
		spaceID, u.ID, isAdmin, listFilter, from, to)
	if err != nil {
		dbFail(r, "timeline", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	items := []timelineItem{}
	inWindow := map[int64]bool{}
	for rows.Next() {
		var (
			it              timelineItem
			startAt, dueAt  *time.Time
			progress        *int
			subDone, subAll int
			perm            *string
		)
		if rows.Scan(&it.ID, &it.ListID, &it.ListName, &it.ParentID, &it.Title, &it.Status, &it.Priority,
			&it.Assignee, &startAt, &dueAt, &progress, &subDone, &subAll,
			&it.BlockedBy, &it.CompletedAt, &perm) != nil {
			continue
		}
		it.CanEdit = isAdmin || (perm != nil && permAtLeast(*perm, "editor"))
		switch {
		case startAt != nil && dueAt != nil:
			it.Start, it.End = *startAt, *dueAt
		case dueAt != nil:
			// Deadline only: a one-day bar landing on the due date.
			it.Start, it.End, it.Implied = dueAt.AddDate(0, 0, -1), *dueAt, true
		default:
			// Start only: open-ended, drawn to the edge of the window.
			it.Start, it.End, it.Implied = *startAt, to, true
		}
		// Progress mirrors the rules used elsewhere: a manual override wins, otherwise it's
		// derived from subtasks, and a completed task always reads 100%.
		switch {
		case it.CompletedAt != nil:
			it.Progress = 100
		case progress != nil:
			it.Progress = *progress
		case subAll > 0:
			it.Progress = subDone * 100 / subAll
		}
		it.Done = it.CompletedAt != nil
		it.Overdue = it.CompletedAt == nil && dueAt != nil && dueAt.Before(now)
		items = append(items, it)
		inWindow[it.ID] = true
	}

	// Dependency arrows, restricted to pairs that are both on screen.
	links := []timelineLink{}
	for _, it := range items {
		for _, dep := range it.BlockedBy {
			if inWindow[dep] {
				links = append(links, timelineLink{From: dep, To: it.ID})
			}
		}
	}

	// Tasks with no dates at all can't be placed on a chart, but silently omitting them would
	// make the Timeline look like the whole project — so they're counted and offered as a
	// "schedule these" prompt in the UI.
	var unscheduled int
	_ = a.DB.Pool.QueryRow(r.Context(), `
		SELECT count(*)::int
		FROM tasks t
		JOIN lists l ON l.id = t.list_id
		LEFT JOIN list_members lm ON lm.list_id = l.id AND lm.user_id = $2
		WHERE l.space_id = $1
		  AND t.archived_at IS NULL AND l.archived_at IS NULL AND t.completed_at IS NULL
		  AND ($3 OR lm.user_id IS NOT NULL OR l.is_private = false)
		  AND ($4 = 0 OR t.list_id = $4)
		  AND t.start_at IS NULL AND t.due_at IS NULL`,
		spaceID, u.ID, u.IsAdmin(), listFilter).Scan(&unscheduled)

	writeJSON(w, http.StatusOK, map[string]any{
		"from":        from.Format("2006-01-02"),
		"to":          to.Format("2006-01-02"),
		"items":       items,
		"links":       links,
		"unscheduled": unscheduled,
	})
}
