package api

import (
	"errors"
	"net/http"
)

// POST /api/spaces/{id}/duplicate {name?}
//
// Owner only. Copying a space clones other people's lists inside it, which is not something a
// plain member should be able to do to a shared space.
func (a *API) handleDuplicateSpace(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	srcID, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if a.spaceRole(r, u.ID, u.IsAdmin(), srcID) != "owner" {
		errJSON(w, http.StatusForbidden, "only the space owner can copy it")
		return
	}
	ctx := r.Context()
	// A copy is a new space, so it goes through the same gate as creating one: an instance where
	// only admins may create spaces must not hand out an unlimited supply of them via "copy".
	if !u.IsAdmin() && a.DB.Setting(ctx, "policy.users.can_create_spaces", "true") == "false" {
		errJSON(w, http.StatusForbidden, "only an administrator can create spaces on this server")
		return
	}
	var in struct {
		Name *string `json:"name"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	var srcName string
	if a.DB.Pool.QueryRow(ctx,
		`SELECT name FROM spaces WHERE id=$1 AND archived_at IS NULL`, srcID).Scan(&srcName) != nil {
		errJSON(w, http.StatusNotFound, "space not found")
		return
	}
	name := srcName
	if in.Name != nil && *in.Name != "" {
		name = *in.Name
	}

	tx, err := a.DB.Pool.Begin(ctx)
	if err != nil {
		dbFail(r, "duplicate space: begin", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// settings carries the workflow, custom fields and Pulse configuration. Those are the shape
	// of the space rather than its content, and a copy with the default workflow would not match
	// the statuses on the tasks being copied into it.
	var newID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO spaces(name, owner_id, settings)
		SELECT $1, $2, settings FROM spaces WHERE id=$3
		RETURNING id`, name, u.ID, srcID).Scan(&newID)
	if err != nil {
		dbFail(r, "duplicate space: insert", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO space_members(space_id, user_id, role) VALUES($1,$2,'owner')
		 ON CONFLICT (space_id, user_id) DO NOTHING`, newID, u.ID); err != nil {
		dbFail(r, "duplicate space: owner membership", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}

	// Only the lists the caller can actually see. A private list belonging to another member is
	// invisible to them in the original space and must stay invisible in the copy — otherwise
	// "copy space" becomes a way to read everyone's private lists.
	rows, err := tx.Query(ctx, `
		SELECT id, name FROM lists
		WHERE space_id=$1 AND archived_at IS NULL
		  AND ($2 OR is_private = FALSE
		       OR EXISTS (SELECT 1 FROM list_members lm WHERE lm.list_id = lists.id AND lm.user_id = $3))
		ORDER BY position, id`, srcID, u.IsAdmin(), u.ID)
	if err != nil {
		dbFail(r, "duplicate space: read lists", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	type srcList struct {
		id   int64
		name string
	}
	var lists []srcList
	for rows.Next() {
		var l srcList
		if err := rows.Scan(&l.id, &l.name); err != nil {
			rows.Close()
			dbFail(r, "duplicate space: scan lists", err)
			errJSON(w, http.StatusInternalServerError, "database error")
			return
		}
		lists = append(lists, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		dbFail(r, "duplicate space: read lists", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}

	// List names are copied verbatim: the "(copy)" suffix belongs on the space, and repeating it
	// on every list inside would be noise.
	//
	// keepNoteLink is false because notes are not copied — a task in the new space would
	// otherwise point at a note living in the old one.
	total := 0
	for _, l := range lists {
		_, copied, err := duplicateListTx(ctx, tx, l.id, newID, l.name, u.ID, false)
		if err != nil {
			if errors.Is(err, errTooManyTasks) {
				errJSON(w, http.StatusRequestEntityTooLarge, "a list in this space has too many tasks to copy")
				return
			}
			dbFail(r, "duplicate space: copy list", err)
			errJSON(w, http.StatusInternalServerError, "database error")
			return
		}
		total += copied
	}
	if err := tx.Commit(ctx); err != nil {
		dbFail(r, "duplicate space: commit", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": newID, "lists": len(lists), "tasks": total})
}
