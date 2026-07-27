package api

// Inbox (spec section 12: «Входящие» и быстрый ввод).
//
// A single cross-space triage list: everything assigned to me that still needs a decision,
// plus tasks I created that nobody has picked up. Distinct from "My tasks", which is a
// scheduled-work view sorted by deadline — the Inbox is specifically about what hasn't been
// dealt with yet, so it surfaces the *unscheduled* and *unassigned* items that a deadline-
// sorted list pushes to the bottom or hides entirely.
//
// Each item carries a `reason` so the UI can group them and the user understands why something
// is asking for attention rather than facing one undifferentiated pile.

import (
	"net/http"
	"time"
)

// GET /api/inbox — tasks needing triage, across every space the caller can see.
func (a *API) handleInbox(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}

	// reason is computed in SQL so ordering can key off it directly:
	//   mentioned  — someone @mentioned me in a comment and I'm not the assignee
	//   assigned   — assigned to me, no deadline set (nothing schedules it, easy to lose)
	//   unassigned — I created it, nobody owns it
	//   review     — assigned to me and waiting on review
	//
	// The mention test is a POSIX match on the pattern built by mentionSQLPattern, not a LIKE:
	// see mentions.go for why (`bob` used to collect every `@bobby`, and an underscore in a
	// username was a wildcard).
	//
	// Visibility mirrors the rule used everywhere else: admins see all, others need list
	// membership or a non-private list in a space they belong to.
	rows, err := a.DB.Pool.Query(r.Context(), `
		WITH visible AS (
			SELECT t.id, t.list_id, t.title, t.status, COALESCE(t.priority,'normal') AS priority,
					t.due_at, t.created_at,
				t.assignee_id, t.creator_id, l.name AS list_name, l.space_id, s.name AS space_name
			FROM tasks t
			JOIN lists l ON l.id = t.list_id
			JOIN spaces s ON s.id = l.space_id
			LEFT JOIN list_members lm ON lm.list_id = l.id AND lm.user_id = $1
			LEFT JOIN space_members sm ON sm.space_id = l.space_id AND sm.user_id = $1
			WHERE t.archived_at IS NULL AND l.archived_at IS NULL AND s.archived_at IS NULL
			  AND t.completed_at IS NULL
			  AND ($2 OR lm.user_id IS NOT NULL OR (l.is_private = false AND sm.user_id IS NOT NULL))
		)
		SELECT v.id, v.list_id, v.list_name, v.space_id, v.space_name, v.title, v.status,
			v.priority, v.due_at, v.created_at,
			CASE
				WHEN v.status = 'review' AND v.assignee_id = $1 THEN 'review'
				WHEN v.assignee_id = $1 AND v.due_at IS NULL     THEN 'assigned'
				WHEN v.assignee_id IS NULL AND v.creator_id = $1 THEN 'unassigned'
				ELSE 'mentioned'
			END AS reason
		FROM visible v
		WHERE (v.status = 'review' AND v.assignee_id = $1)
		   OR (v.assignee_id = $1 AND v.due_at IS NULL)
		   OR (v.assignee_id IS NULL AND v.creator_id = $1)
		   OR EXISTS (
		        SELECT 1 FROM comments c
		        WHERE c.task_id = v.id AND c.deleted_at IS NULL
		          AND c.body ~ $3
		          AND c.author_id <> $1
		          AND COALESCE(v.assignee_id, 0) <> $1
		      )
		ORDER BY v.created_at DESC
		LIMIT 200`, u.ID, u.IsAdmin(), mentionSQLPattern(u.Username))
	if err != nil {
		dbFail(r, "inbox", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	type inboxItem struct {
		ID        int64      `json:"id"`
		ListID    int64      `json:"list_id"`
		ListName  string     `json:"list_name"`
		SpaceID   int64      `json:"space_id"`
		SpaceName string     `json:"space_name"`
		Title     string     `json:"title"`
		Status    string     `json:"status"`
		Priority  string     `json:"priority"`
		DueAt     *time.Time `json:"due_at"`
		CreatedAt time.Time  `json:"created_at"`
		Reason    string     `json:"reason"`
	}
	items := []inboxItem{}
	counts := map[string]int{}
	for rows.Next() {
		var it inboxItem
		if rows.Scan(&it.ID, &it.ListID, &it.ListName, &it.SpaceID, &it.SpaceName, &it.Title,
			&it.Status, &it.Priority, &it.DueAt, &it.CreatedAt, &it.Reason) != nil {
			continue
		}
		items = append(items, it)
		counts[it.Reason]++
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "counts": counts})
}
