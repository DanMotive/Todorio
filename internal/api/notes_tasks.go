package api

// Turning a note into tasks.
//
// Notes and tasks have lived in the same space since 0004 with no connection between them at
// all. In practice a note is where a meeting gets written down, and the decisions in it are
// re-typed by hand into tasks afterwards - or, more often, not re-typed at all. This is the
// missing edge: extract the checklist lines from a note and create real tasks from them, with a
// link back to where they came from.
//
// The link is one-way and loose on purpose. Editing the note later does not touch the tasks, and
// deleting the note does not delete them (see 0014's ON DELETE SET NULL). A note is a record of
// a conversation; a task is committed work. Keeping them in sync would mean one could silently
// rewrite the other.

import (
	"net/http"
	"strings"

	"github.com/DanMotive/Todorio/internal/events"
)

// noteTaskMax caps one extraction. A note is a page of prose, not a bulk-import format; anything
// past this is a sign the wrong tool is being used, and inserting hundreds of rows from a single
// click is worse than refusing.
const noteTaskMax = 100

// parseNoteTaskLines pulls candidate task titles out of a note's Markdown body.
//
// Only unchecked checkbox lines qualify: "- [ ] ship the thing". A checked box is work the
// author has already marked finished, and creating an open task from it would be actively wrong.
// Plain bullets are deliberately ignored too - most bullets in a note are context, not
// commitments, and a feature that turns every bullet into a task produces a list nobody wants.
//
// Kept pure (no request, no database) so the parsing rules can be tested directly.
func parseNoteTaskLines(body string) []string {
	var out []string
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		// Accept both bullet markers Markdown allows, and tolerate the capital "[X]" form when
		// checking for the already-done case below.
		for _, marker := range []string{"- ", "* ", "+ "} {
			if !strings.HasPrefix(line, marker) {
				continue
			}
			rest := strings.TrimSpace(strings.TrimPrefix(line, marker))
			if !strings.HasPrefix(rest, "[ ]") {
				break
			}
			title := strings.TrimSpace(strings.TrimPrefix(rest, "[ ]"))
			if title != "" {
				out = append(out, title)
			}
			break
		}
		if len(out) >= noteTaskMax {
			break
		}
	}
	return out
}

// GET /api/notes/{id}/tasks - what has already been extracted from this note.
//
// Without this the button would be blind: a user who clicks twice has no way to see that the
// tasks already exist, and would just make duplicates.
func (a *API) handleNoteTasks(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	noteID, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var spaceID int64
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT space_id FROM notes WHERE id=$1`, noteID).Scan(&spaceID) != nil {
		errJSON(w, http.StatusNotFound, "note not found")
		return
	}
	if a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "no access")
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT t.id, t.list_id, t.title, t.status, t.completed_at IS NOT NULL
		FROM tasks t
		JOIN lists l ON l.id = t.list_id
		WHERE t.source_note_id = $1
		  AND t.archived_at IS NULL
		  AND ($2 OR l.is_private = false OR l.id IN (
				SELECT list_id FROM list_members WHERE user_id = $3
		  ))
		ORDER BY t.id`, noteID, u.IsAdmin(), u.ID)
	if err != nil {
		dbFail(r, "note tasks", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	tasks := []map[string]any{}
	for rows.Next() {
		var id, listID int64
		var title, status string
		var done bool
		if rows.Scan(&id, &listID, &title, &status, &done) == nil {
			tasks = append(tasks, map[string]any{
				"id": id, "list_id": listID, "title": title, "status": status, "done": done,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

// POST /api/notes/{id}/tasks {list_id, titles?}
//
// With no titles supplied the note's own unchecked checkboxes are used, which is the one-click
// path. An explicit titles array is what the UI sends when the user has ticked a subset in a
// confirmation dialog - the server does not assume the client agreed with its own parsing.
func (a *API) handleCreateTasksFromNote(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	noteID, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in struct {
		ListID int64    `json:"list_id"`
		Titles []string `json:"titles"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}

	var spaceID int64
	var noteTitle, body string
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT space_id, title, body FROM notes WHERE id=$1 AND archived_at IS NULL`, noteID).
		Scan(&spaceID, &noteTitle, &body) != nil {
		errJSON(w, http.StatusNotFound, "note not found")
		return
	}
	role := a.spaceRole(r, u.ID, u.IsAdmin(), spaceID)
	if role != "owner" && role != "member" {
		errJSON(w, http.StatusForbidden, "no permission")
		return
	}

	// The target list must be in the same space as the note. Otherwise this endpoint would be a
	// way to write into any list whose id you can guess, using note access as the only check.
	var listSpace int64
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT space_id FROM lists WHERE id=$1 AND archived_at IS NULL`, in.ListID).Scan(&listSpace) != nil {
		errJSON(w, http.StatusNotFound, "list not found")
		return
	}
	if listSpace != spaceID {
		errJSON(w, http.StatusBadRequest, "the list belongs to a different space")
		return
	}
	if !permAtLeast(a.listPermission(r, u, in.ListID), "editor") {
		errJSON(w, http.StatusForbidden, "no permission to add tasks to that list")
		return
	}

	titles := in.Titles
	if len(titles) == 0 {
		titles = parseNoteTaskLines(body)
	}
	cleaned := []string{}
	for _, t := range titles {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		cleaned = append(cleaned, t)
		if len(cleaned) >= noteTaskMax {
			break
		}
	}
	if len(cleaned) == 0 {
		errJSON(w, http.StatusBadRequest, "nothing to create - the note has no unchecked checklist items")
		return
	}

	// One transaction: a half-extracted note leaves the user unable to tell which lines made it
	// across, and clicking again would duplicate the ones that did.
	tx, err := a.DB.Pool.Begin(r.Context())
	if err != nil {
		dbFail(r, "note to tasks begin", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	// The description points back at the note by name rather than by link: notes have no stable
	// public URL, and a title someone can search for survives better than a path that may change.
	description := "From note: " + noteTitle
	ids := []int64{}
	for _, title := range cleaned {
		var id int64
		if err := tx.QueryRow(r.Context(), `
			INSERT INTO tasks(list_id, title, description, status, creator_id, source_note_id)
			VALUES($1,$2,$3,'open',$4,$5) RETURNING id`,
			in.ListID, title, description, u.ID, noteID).Scan(&id); err != nil {
			dbFail(r, "note to tasks insert", err)
			errJSON(w, http.StatusInternalServerError, "could not create the tasks")
			return
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(r.Context()); err != nil {
		dbFail(r, "note to tasks commit", err)
		errJSON(w, http.StatusInternalServerError, "could not save the tasks")
		return
	}

	a.publishToListMembers(r, in.ListID, events.Event{
		Type: "task.created",
		Data: map[string]any{"list_id": in.ListID},
	})
	writeJSON(w, http.StatusCreated, map[string]any{"created": len(ids), "task_ids": ids})
}
