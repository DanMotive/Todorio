// Archive safety net (spec section 11): tasks/lists/spaces move to a soft "archived" state and
// are only permanently deleted after policy.archive.retention_days (worker.cleanupArchive). This
// file adds the three things that were missing around that: viewing what's archived, restoring an
// item (clearing the countdown entirely), and a deliberate, root-only permanent delete for anyone
// who really does want it gone immediately. Task restore/permanent-delete live in tasks.go next to
// handleArchiveTask; this file covers lists, spaces, and the read side for both.
package api

import (
	"net/http"
)

// GET /api/spaces/{id}/archive — archived lists in this space, plus tasks that were archived
// individually (their list is still live). Tasks whose list is *also* archived aren't listed
// separately here — restoring the list brings them back too (see handleRestoreList), so surfacing
// them twice would be confusing. Subtasks are likewise omitted; they follow their parent task.
func (a *API) handleSpaceArchive(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "no access to the space")
		return
	}
	lists := []map[string]any{}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT id, name, archived_at, archived_by FROM lists
		WHERE space_id=$1 AND archived_at IS NOT NULL ORDER BY archived_at DESC`, spaceID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var name string
			var archivedAt any
			var archivedBy *int64
			if rows.Scan(&id, &name, &archivedAt, &archivedBy) == nil {
				lists = append(lists, map[string]any{"id": id, "name": name, "archived_at": archivedAt, "archived_by": archivedBy})
			}
		}
	}
	tasks := []map[string]any{}
	trows, err := a.DB.Pool.Query(r.Context(), `
		SELECT t.id, t.title, t.list_id, l.name, t.archived_at, t.archived_by
		FROM tasks t JOIN lists l ON l.id = t.list_id
		WHERE l.space_id=$1 AND t.archived_at IS NOT NULL AND l.archived_at IS NULL AND t.parent_id IS NULL
		ORDER BY t.archived_at DESC`, spaceID)
	if err == nil {
		defer trows.Close()
		for trows.Next() {
			var id, listID int64
			var title, listName string
			var archivedAt any
			var archivedBy *int64
			if trows.Scan(&id, &title, &listID, &listName, &archivedAt, &archivedBy) == nil {
				tasks = append(tasks, map[string]any{
					"id": id, "title": title, "list_id": listID, "list_name": listName,
					"archived_at": archivedAt, "archived_by": archivedBy,
				})
			}
		}
	}
	retentionDays := a.DB.Setting(r.Context(), "policy.archive.retention_days", "30")
	writeJSON(w, http.StatusOK, map[string]any{"lists": lists, "tasks": tasks, "retention_days": retentionDays})
}

// GET /api/archive/spaces — archived spaces the user owns (root/admin see all archived spaces,
// for oversight — they're also the only ones who can permanently delete any of them).
func (a *API) handleArchivedSpaces(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT s.id, s.name, s.archived_at, s.archived_by
		FROM spaces s
		LEFT JOIN space_members m ON m.space_id = s.id AND m.user_id = $1 AND m.role = 'owner'
		WHERE s.archived_at IS NOT NULL AND ($2 OR m.user_id IS NOT NULL)
		ORDER BY s.archived_at DESC`, u.ID, u.IsAdmin())
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	spaces := []map[string]any{}
	for rows.Next() {
		var id int64
		var name string
		var archivedAt any
		var archivedBy *int64
		if rows.Scan(&id, &name, &archivedAt, &archivedBy) == nil {
			spaces = append(spaces, map[string]any{"id": id, "name": name, "archived_at": archivedAt, "archived_by": archivedBy})
		}
	}
	retentionDays := a.DB.Setting(r.Context(), "policy.archive.retention_days", "30")
	writeJSON(w, http.StatusOK, map[string]any{"spaces": spaces, "retention_days": retentionDays})
}

// POST /api/lists/{id}/restore — clears the archive countdown for the list AND every task inside
// it (archiving a list unconditionally archives its tasks too — see handleArchiveList — so
// restoring it unconditionally brings all of them back for the same reason: leaving some tasks
// stuck in a "restored list, still-archived task" limbo would be confusing and hard to discover).
func (a *API) handleRestoreList(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil || !permAtLeast(a.listPermission(r, u, id), "owner") {
		errJSON(w, http.StatusForbidden, "list owner permission required")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE lists SET archived_at=NULL, archived_by=NULL WHERE id=$1`, id)
	_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE tasks SET archived_at=NULL, archived_by=NULL WHERE list_id=$1 AND archived_at IS NOT NULL`, id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/lists/{id}/permanent — irreversible, root only, only for already-archived lists.
// Cascades to its tasks via the tasks.list_id ON DELETE CASCADE foreign key.
func (a *API) handleDeleteListPermanent(w http.ResponseWriter, r *http.Request) {
	u := a.requireAdmin(w, r)
	if u == nil {
		return
	}
	if u.Role != "root" {
		errJSON(w, http.StatusForbidden, "root permission required")
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(), `DELETE FROM lists WHERE id=$1 AND archived_at IS NOT NULL`, id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if tag.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "list not found or not archived — archive it first")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/spaces/{id}/restore — archiving a space never touched its lists/tasks (see
// handleArchiveSpace), so restoring it is just clearing the space's own countdown.
func (a *API) handleRestoreSpace(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), id) != "owner" {
		errJSON(w, http.StatusForbidden, "space owner permission required")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE spaces SET archived_at=NULL, archived_by=NULL WHERE id=$1`, id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/spaces/{id}/permanent — irreversible, root only, only for already-archived spaces.
// Cascades to lists (and, transitively, their tasks) via ON DELETE CASCADE foreign keys.
func (a *API) handleDeleteSpacePermanent(w http.ResponseWriter, r *http.Request) {
	u := a.requireAdmin(w, r)
	if u == nil {
		return
	}
	if u.Role != "root" {
		errJSON(w, http.StatusForbidden, "root permission required")
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(), `DELETE FROM spaces WHERE id=$1 AND archived_at IS NOT NULL`, id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if tag.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "space not found or not archived — archive it first")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
