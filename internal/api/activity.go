package api

// Activity feed: a merged, recent-first view of what happened in a space
// (created/completed tasks, new comments) — the frontend sorts by "at".

import "net/http"

// GET /api/spaces/{id}/activity
func (a *API) handleSpaceActivity(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "no access to the space")
		return
	}
	events := []map[string]any{}

	createdRows, err := a.DB.Pool.Query(r.Context(), `
		SELECT t.id, t.title, u.username, t.created_at
		FROM tasks t JOIN users u ON u.id = t.creator_id
		WHERE t.list_id IN (SELECT id FROM lists WHERE space_id=$1) AND t.archived_at IS NULL
		ORDER BY t.created_at DESC LIMIT 30`, spaceID)
	if err == nil {
		for createdRows.Next() {
			var id int64
			var title, username string
			var at any
			if createdRows.Scan(&id, &title, &username, &at) == nil {
				events = append(events, map[string]any{"type": "task_created", "task_id": id, "title": title, "by": username, "at": at})
			}
		}
		createdRows.Close()
	}

	doneRows, err := a.DB.Pool.Query(r.Context(), `
		SELECT t.id, t.title, COALESCE(u.username,''), t.completed_at
		FROM tasks t LEFT JOIN users u ON u.id = t.assignee_id
		WHERE t.list_id IN (SELECT id FROM lists WHERE space_id=$1) AND t.completed_at IS NOT NULL
		ORDER BY t.completed_at DESC LIMIT 30`, spaceID)
	if err == nil {
		for doneRows.Next() {
			var id int64
			var title, username string
			var at any
			if doneRows.Scan(&id, &title, &username, &at) == nil {
				events = append(events, map[string]any{"type": "task_completed", "task_id": id, "title": title, "by": username, "at": at})
			}
		}
		doneRows.Close()
	}

	commentRows, err := a.DB.Pool.Query(r.Context(), `
		SELECT c.id, c.task_id, t.title, u.username, c.created_at
		FROM comments c
		JOIN tasks t ON t.id = c.task_id
		JOIN users u ON u.id = c.author_id
		WHERE t.list_id IN (SELECT id FROM lists WHERE space_id=$1) AND c.deleted_at IS NULL
		ORDER BY c.created_at DESC LIMIT 30`, spaceID)
	if err == nil {
		for commentRows.Next() {
			var id, taskID int64
			var title, username string
			var at any
			if commentRows.Scan(&id, &taskID, &title, &username, &at) == nil {
				events = append(events, map[string]any{"type": "comment", "task_id": taskID, "title": title, "by": username, "at": at})
			}
		}
		commentRows.Close()
	}

	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}
