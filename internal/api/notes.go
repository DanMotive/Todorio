package api

// Notes: simple Markdown pages inside a space (optionally scoped to one list) — a personal/team
// notebook, separate from tasks. Stored as plain Markdown text; the frontend renders it.

import (
	"net/http"
	"time"
)

// GET /api/spaces/{id}/notes
func (a *API) handleListNotes(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "no access to the space")
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT id, list_id, title, created_by, created_at, updated_at
		FROM notes WHERE space_id=$1 AND archived_at IS NULL ORDER BY updated_at DESC`, spaceID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	defer rows.Close()
	notes := []map[string]any{}
	for rows.Next() {
		var id int64
		var listID *int64
		var title string
		var createdBy int64
		var createdAt, updatedAt time.Time
		if rows.Scan(&id, &listID, &title, &createdBy, &createdAt, &updatedAt) == nil {
			notes = append(notes, map[string]any{
				"id": id, "list_id": listID, "title": title, "created_by": createdBy,
				"created_at": createdAt, "updated_at": updatedAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": notes})
}

// GET /api/notes/{id}
func (a *API) handleGetNote(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var spaceID int64
	var listID *int64
	var title, body string
	var createdBy int64
	var createdAt, updatedAt time.Time
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT space_id, list_id, title, body, created_by, created_at, updated_at
		 FROM notes WHERE id=$1 AND archived_at IS NULL`, id).
		Scan(&spaceID, &listID, &title, &body, &createdBy, &createdAt, &updatedAt) != nil {
		errJSON(w, http.StatusNotFound, "note not found")
		return
	}
	if a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "no access")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"note": map[string]any{
		"id": id, "space_id": spaceID, "list_id": listID, "title": title, "body": body,
		"created_by": createdBy, "created_at": createdAt, "updated_at": updatedAt,
	}})
}

// POST /api/spaces/{id}/notes {title?, list_id?, body?}
func (a *API) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	role := a.spaceRole(r, u.ID, u.IsAdmin(), spaceID)
	if err != nil || (role != "owner" && role != "member") {
		errJSON(w, http.StatusForbidden, "no permission to create notes")
		return
	}
	var in struct {
		Title  *string `json:"title"`
		ListID *int64  `json:"list_id"`
		Body   *string `json:"body"`
	}
	_ = readJSON(r, &in)
	title := "Untitled"
	if in.Title != nil && *in.Title != "" {
		title = *in.Title
	}
	body := ""
	if in.Body != nil {
		body = *in.Body
	}
	var id int64
	err = a.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO notes(space_id, list_id, title, body, created_by) VALUES($1,$2,$3,$4,$5) RETURNING id`,
		spaceID, in.ListID, title, body, u.ID).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// PATCH /api/notes/{id} {title?, body?}
func (a *API) handleUpdateNote(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var spaceID int64
	if a.DB.Pool.QueryRow(r.Context(), `SELECT space_id FROM notes WHERE id=$1 AND archived_at IS NULL`, id).Scan(&spaceID) != nil {
		errJSON(w, http.StatusNotFound, "note not found")
		return
	}
	role := a.spaceRole(r, u.ID, u.IsAdmin(), spaceID)
	if role != "owner" && role != "member" {
		errJSON(w, http.StatusForbidden, "no permission")
		return
	}
	var in struct {
		Title *string `json:"title"`
		Body  *string `json:"body"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	_, err = a.DB.Pool.Exec(r.Context(),
		`UPDATE notes SET title=COALESCE($2,title), body=COALESCE($3,body), updated_at=now() WHERE id=$1`,
		id, in.Title, in.Body)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/notes/{id}
func (a *API) handleArchiveNote(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var spaceID int64
	if a.DB.Pool.QueryRow(r.Context(), `SELECT space_id FROM notes WHERE id=$1`, id).Scan(&spaceID) != nil {
		errJSON(w, http.StatusNotFound, "note not found")
		return
	}
	role := a.spaceRole(r, u.ID, u.IsAdmin(), spaceID)
	if role != "owner" && role != "member" {
		errJSON(w, http.StatusForbidden, "no permission")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE notes SET archived_at=now() WHERE id=$1`, id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
