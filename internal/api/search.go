package api

// Global search across tasks, notes, and comments the user has access to.

import "net/http"

// GET /api/search?q=...
func (a *API) handleSearch(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		errJSON(w, http.StatusBadRequest, "query must be at least 2 characters")
		return
	}
	like := "%" + q + "%"
	results := []map[string]any{}

	taskRows, err := a.DB.Pool.Query(r.Context(), `
		SELECT t.id, t.list_id, t.title
		FROM tasks t
		WHERE t.archived_at IS NULL AND (t.title ILIKE $1 OR t.description ILIKE $1)
			AND ($2 OR t.list_id IN (SELECT list_id FROM list_members WHERE user_id=$3)
				OR t.list_id IN (SELECT id FROM lists WHERE is_private=false))
		ORDER BY t.updated_at DESC LIMIT 20`, like, u.IsAdmin(), u.ID)
	if err == nil {
		for taskRows.Next() {
			var id, listID int64
			var title string
			if taskRows.Scan(&id, &listID, &title) == nil {
				results = append(results, map[string]any{"type": "task", "id": id, "list_id": listID, "title": title})
			}
		}
		taskRows.Close()
	}

	noteRows, err := a.DB.Pool.Query(r.Context(), `
		SELECT n.id, n.space_id, n.title
		FROM notes n
		WHERE n.archived_at IS NULL AND (n.title ILIKE $1 OR n.body ILIKE $1)
			AND ($2 OR n.space_id IN (SELECT space_id FROM space_members WHERE user_id=$3))
		ORDER BY n.updated_at DESC LIMIT 20`, like, u.IsAdmin(), u.ID)
	if err == nil {
		for noteRows.Next() {
			var id, spaceID int64
			var title string
			if noteRows.Scan(&id, &spaceID, &title) == nil {
				results = append(results, map[string]any{"type": "note", "id": id, "space_id": spaceID, "title": title})
			}
		}
		noteRows.Close()
	}

	commentRows, err := a.DB.Pool.Query(r.Context(), `
		SELECT c.id, c.task_id, t.title, c.body
		FROM comments c JOIN tasks t ON t.id = c.task_id
		WHERE c.deleted_at IS NULL AND t.archived_at IS NULL AND c.body ILIKE $1
			AND ($2 OR t.list_id IN (SELECT list_id FROM list_members WHERE user_id=$3)
				OR t.list_id IN (SELECT id FROM lists WHERE is_private=false))
		ORDER BY c.created_at DESC LIMIT 20`, like, u.IsAdmin(), u.ID)
	if err == nil {
		for commentRows.Next() {
			var id, taskID int64
			var taskTitle, body string
			if commentRows.Scan(&id, &taskID, &taskTitle, &body) == nil {
				results = append(results, map[string]any{
					"type": "comment", "id": id, "task_id": taskID, "task_title": taskTitle, "snippet": body,
				})
			}
		}
		commentRows.Close()
	}

	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
