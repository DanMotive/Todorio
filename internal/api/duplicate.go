package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// A copy runs as one transaction and one statement per task, so an accidental attempt to clone a
// pathologically large list can't hold a write transaction open for minutes. The cap is far above
// any realistic list; it exists to fail fast with a clear message instead of timing out.
const maxDuplicateTasks = 5000

var errTooManyTasks = errors.New("too many tasks to copy")

// copyTasks copies every live task of srcList into dstList.
//
// Two passes on purpose. Tasks reference each other through parent_id (subtasks) and blocked_by
// (dependencies), so the new rows have to exist before those links can point anywhere: the first
// pass inserts every task with its links left empty, the second rewrites them to the copies.
// A single pass would either point the copies back at the originals — so ticking a subtask in the
// copy would move a task in the source list — or silently drop the structure.
//
// The insert is an INSERT ... SELECT per task rather than a Go struct round-trip. tasks has 20+
// columns of mixed types (JSONB, arrays, nullable timestamps); listing them in SQL and never
// materialising them in Go means a column added by a later migration cannot be silently dropped
// from copies by a scan that forgot about it.
func copyTasks(ctx context.Context, tx pgx.Tx, srcList, dstList, actor int64, keepNoteLink bool) (int, error) {
	type srcTask struct {
		id      int64
		parent  *int64
		blocked []int64
	}
	rows, err := tx.Query(ctx, `
		SELECT id, parent_id, blocked_by FROM tasks
		WHERE list_id=$1 AND archived_at IS NULL
		ORDER BY position, id`, srcList)
	if err != nil {
		return 0, err
	}
	var src []srcTask
	for rows.Next() {
		var t srcTask
		if err := rows.Scan(&t.id, &t.parent, &t.blocked); err != nil {
			rows.Close()
			return 0, err
		}
		src = append(src, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(src) > maxDuplicateTasks {
		return 0, errTooManyTasks
	}

	// review_state / review_by / review_at / review_note are deliberately not copied: a review
	// decision is a statement about a specific piece of work by a specific person. Carrying
	// "accepted by Ivan" into a fresh copy nobody has looked at would be a false record.
	// creator_id becomes the person doing the copying — they created these rows, however the
	// text got there.
	idMap := make(map[int64]int64, len(src))
	for _, t := range src {
		var newID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO tasks (list_id, title, description, status, priority, assignee_id,
				start_at, due_at, recurrence, progress, weight, custom_fields, position,
				completed_at, creator_id, source_note_id)
			SELECT $1, title, description, status, priority, assignee_id,
				start_at, due_at, recurrence, progress, weight, custom_fields, position,
				completed_at, $2, CASE WHEN $3 THEN source_note_id ELSE NULL END
			FROM tasks WHERE id=$4
			RETURNING id`, dstList, actor, keepNoteLink, t.id).Scan(&newID)
		if err != nil {
			return 0, err
		}
		idMap[t.id] = newID
	}

	for _, t := range src {
		var parent *int64
		if t.parent != nil {
			// A subtask whose parent was archived has no copy to hang off, so it becomes a
			// top-level task rather than pointing at the original parent in the source list.
			if mapped, ok := idMap[*t.parent]; ok {
				parent = &mapped
			}
		}
		// Dependencies on tasks outside this list are dropped, not rewritten. "Blocked by
		// something in another list" is a fact about the original; keeping the reference would
		// make a copy in a different space depend on work its members may not even see.
		blocked := make([]int64, 0, len(t.blocked))
		for _, b := range t.blocked {
			if mapped, ok := idMap[b]; ok {
				blocked = append(blocked, mapped)
			}
		}
		if parent == nil && len(blocked) == 0 {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE tasks SET parent_id=$2, blocked_by=$3 WHERE id=$1`,
			idMap[t.id], parent, blocked); err != nil {
			return 0, err
		}
	}
	return len(src), nil
}

// duplicateListTx creates the copy of one list inside an existing transaction. Shared with the
// whole-space copy, which needs many lists to succeed or fail together.
func duplicateListTx(ctx context.Context, tx pgx.Tx, srcID, spaceID int64, name string, actor int64, keepNoteLink bool) (int64, int, error) {
	var newID int64
	err := tx.QueryRow(ctx, `
		INSERT INTO lists(space_id, name, is_private, settings, position)
		SELECT $1, $2, is_private, settings,
			COALESCE((SELECT max(position) FROM lists WHERE space_id=$1 AND archived_at IS NULL), 0) + 1
		FROM lists WHERE id=$3
		RETURNING id`, spaceID, name, srcID).Scan(&newID)
	if err != nil {
		return 0, 0, err
	}
	// The person copying owns the copy. Other members of the source list are not carried over:
	// sharing is a decision per list, and a copy silently inheriting an audience is how content
	// leaks into a space where those people were never meant to have access.
	if _, err := tx.Exec(ctx,
		`INSERT INTO list_members(list_id, user_id, permission) VALUES($1,$2,'owner')
		 ON CONFLICT (list_id, user_id) DO NOTHING`, newID, actor); err != nil {
		return 0, 0, err
	}
	copied, err := copyTasks(ctx, tx, srcID, newID, actor, keepNoteLink)
	if err != nil {
		return 0, 0, err
	}
	return newID, copied, nil
}

// POST /api/lists/{id}/duplicate {space_id?, name?}
//
// Reading the source needs viewer; writing the copy needs member-or-owner in the target space —
// the same pair of checks a manual "create a list and retype everything" would go through.
func (a *API) handleDuplicateList(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	srcID, err := pathID(r)
	if err != nil || !permAtLeast(a.listPermission(r, u, srcID), "viewer") {
		errJSON(w, http.StatusForbidden, "no access to the list")
		return
	}
	var in struct {
		SpaceID *int64  `json:"space_id"`
		Name    *string `json:"name"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx := r.Context()
	var srcSpace int64
	var srcName string
	if a.DB.Pool.QueryRow(ctx,
		`SELECT space_id, name FROM lists WHERE id=$1 AND archived_at IS NULL`, srcID).Scan(&srcSpace, &srcName) != nil {
		errJSON(w, http.StatusNotFound, "list not found")
		return
	}
	target := srcSpace
	if in.SpaceID != nil {
		target = *in.SpaceID
	}
	// spaceRole caps a globally read-only account at "viewer", so this one check also stops a
	// viewer from writing anything via a copy.
	if role := a.spaceRole(r, u.ID, u.IsAdmin(), target); role != "owner" && role != "member" {
		errJSON(w, http.StatusForbidden, "no permission to create a list in that space")
		return
	}
	if limit := a.intSetting(ctx, "limits.content.lists_per_user", 0); limit > 0 {
		var count int
		_ = a.DB.Pool.QueryRow(ctx, `
			SELECT count(*) FROM lists l JOIN list_members lm ON lm.list_id=l.id
			WHERE lm.user_id=$1 AND lm.permission='owner' AND l.archived_at IS NULL`, u.ID).Scan(&count)
		if count >= limit {
			errJSON(w, http.StatusForbidden, "you've reached the maximum number of lists")
			return
		}
	}
	// The client decides what a copy is called, because "(copy)" has to be in the user's
	// language and the server has no business localising it. Omitting the name keeps the
	// original one — two lists may share a name.
	name := srcName
	if in.Name != nil && *in.Name != "" {
		name = *in.Name
	}

	tx, err := a.DB.Pool.Begin(ctx)
	if err != nil {
		dbFail(r, "duplicate list: begin", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// source_note_id is kept only within the same space: across spaces it would point at a note
	// the copy's audience cannot open.
	newID, copied, err := duplicateListTx(ctx, tx, srcID, target, name, u.ID, target == srcSpace)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		if errors.Is(err, errTooManyTasks) {
			errJSON(w, http.StatusRequestEntityTooLarge, "the list has too many tasks to copy")
			return
		}
		dbFail(r, "duplicate list", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": newID, "space_id": target, "tasks": copied})
}
