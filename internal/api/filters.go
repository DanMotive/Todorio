package api

// Saved filters: reusable query definitions per user, for one list or globally (e.g. "My tasks").
// query is a free-form JSON object interpreted by the frontend (status, priority, assignee_id, label, overdue, ...).

import "net/http"

// GET /api/filters?list_id= — global filters when list_id is omitted.
func (a *API) handleListFilters(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	listIDStr := r.URL.Query().Get("list_id")
	query := `SELECT id, list_id, name, query FROM saved_filters WHERE user_id=$1 AND list_id IS NULL ORDER BY id`
	args := []any{u.ID}
	if listIDStr != "" {
		query = `SELECT id, list_id, name, query FROM saved_filters WHERE user_id=$1 AND list_id=$2 ORDER BY id`
		args = append(args, listIDStr)
	}
	rows, err := a.DB.Pool.Query(r.Context(), query, args...)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	defer rows.Close()
	filters := []map[string]any{}
	for rows.Next() {
		var id int64
		var listID *int64
		var name string
		var q any
		if rows.Scan(&id, &listID, &name, &q) == nil {
			filters = append(filters, map[string]any{"id": id, "list_id": listID, "name": name, "query": q})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"filters": filters})
}

// POST /api/filters {name, list_id?, query}
func (a *API) handleCreateFilter(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var in struct {
		Name   string         `json:"name"`
		ListID *int64         `json:"list_id"`
		Query  map[string]any `json:"query"`
	}
	if err := readJSON(r, &in); err != nil || in.Name == "" {
		errJSON(w, http.StatusBadRequest, "filter name is required")
		return
	}
	var id int64
	err := a.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO saved_filters(user_id, list_id, name, query) VALUES($1,$2,$3,$4) RETURNING id`,
		u.ID, in.ListID, in.Name, in.Query).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// DELETE /api/filters/{id}
func (a *API) handleDeleteFilter(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(), `DELETE FROM saved_filters WHERE id=$1 AND user_id=$2`, id, u.ID)
	if err != nil || tag.RowsAffected() == 0 {
		errJSON(w, http.StatusForbidden, "cannot delete this filter")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
