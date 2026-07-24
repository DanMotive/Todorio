// Task version history (spec section 11): task_versions has been written on every edit since the
// very first pass (see handleUpdateTask's snapshot insert), but nothing ever read it back — the
// data just piled up unused. This file adds the read side and a restore action.
package api

import (
	"encoding/json"
	"net/http"
	"time"
)

// GET /api/tasks/{id}/versions — up to the last 50 snapshots, newest first. Requires at least
// viewer access to the task's list (same bar as reading the task itself).
func (a *API) handleListTaskVersions(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if !a.featureEnabled(r.Context(), "versions") {
		writeJSON(w, http.StatusOK, map[string]any{"versions": []map[string]any{}})
		return
	}
	taskID, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var listID int64
	if a.DB.Pool.QueryRow(r.Context(), `SELECT list_id FROM tasks WHERE id=$1`, taskID).Scan(&listID) != nil {
		errJSON(w, http.StatusNotFound, "task not found")
		return
	}
	if !permAtLeast(a.listPermission(r, u, listID), "viewer") {
		errJSON(w, http.StatusForbidden, "no access")
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT v.id, v.snapshot, v.editor_id, u.username, v.changed_at
		FROM task_versions v JOIN users u ON u.id = v.editor_id
		WHERE v.task_id=$1 ORDER BY v.changed_at DESC LIMIT 50`, taskID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	versions := []map[string]any{}
	for rows.Next() {
		var id, editorID int64
		var snapshot json.RawMessage
		var username string
		var changedAt time.Time
		if rows.Scan(&id, &snapshot, &editorID, &username, &changedAt) == nil {
			versions = append(versions, map[string]any{
				"id": id, "snapshot": snapshot, "editor_id": editorID, "editor": username, "changed_at": changedAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

// snapForRestore is the subset of a task_versions.snapshot (a full to_jsonb(tasks row) dump —
// see handleUpdateTask) that it makes sense to restore. Deliberately excludes id/list_id/
// created_by/created_at/archived_at etc. — restoring a past version brings back what the task
// *said*, not where it lived or who made it.
type snapForRestore struct {
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Status       string          `json:"status"`
	Priority     *string         `json:"priority"`
	AssigneeID   *int64          `json:"assignee_id"`
	DueAt        *time.Time      `json:"due_at"`
	Progress     *int            `json:"progress"`
	Weight       int             `json:"weight"`
	BlockedBy    []int64         `json:"blocked_by"`
	Recurrence   json.RawMessage `json:"recurrence"`
	CustomFields json.RawMessage `json:"custom_fields"`
}

// POST /api/tasks/{id}/versions/{version_id}/restore — applies a past snapshot's editable fields
// back onto the live task. Takes its own "before" snapshot first, so restoring is itself
// undoable, same as any other edit (and shows up in the version list too).
func (a *API) handleRestoreTaskVersion(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if !a.featureEnabled(r.Context(), "versions") {
		errJSON(w, http.StatusForbidden, "version history is disabled on this server")
		return
	}
	taskID, err := pathIDNamed(r, "id")
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	versionID, err := pathIDNamed(r, "version_id")
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid version id")
		return
	}
	var listID int64
	if a.DB.Pool.QueryRow(r.Context(), `SELECT list_id FROM tasks WHERE id=$1`, taskID).Scan(&listID) != nil {
		errJSON(w, http.StatusNotFound, "task not found")
		return
	}
	if !permAtLeast(a.listPermission(r, u, listID), "editor") {
		errJSON(w, http.StatusForbidden, "no permission to edit")
		return
	}
	var rawSnapshot json.RawMessage
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT snapshot FROM task_versions WHERE id=$1 AND task_id=$2`, versionID, taskID).Scan(&rawSnapshot) != nil {
		errJSON(w, http.StatusNotFound, "version not found")
		return
	}
	var snap snapForRestore
	if json.Unmarshal(rawSnapshot, &snap) != nil {
		errJSON(w, http.StatusInternalServerError, "corrupt version snapshot")
		return
	}
	// A stored SQL NULL (e.g. no recurrence) round-trips through to_jsonb/json.Unmarshal as the
	// 4-byte JSON literal "null", not an empty RawMessage — normalize both back to nil/`{}` so we
	// write real SQL NULL / the correct default back, not the literal jsonb value `null`.
	if len(snap.Recurrence) == 0 || string(snap.Recurrence) == "null" {
		snap.Recurrence = nil
	}
	if len(snap.CustomFields) == 0 || string(snap.CustomFields) == "null" {
		snap.CustomFields = json.RawMessage("{}")
	}

	_, _ = a.DB.Pool.Exec(r.Context(), `
		INSERT INTO task_versions(task_id, editor_id, snapshot)
		SELECT id, $2, to_jsonb(t) FROM tasks t WHERE id=$1`, taskID, u.ID)

	_, err = a.DB.Pool.Exec(r.Context(), `
		UPDATE tasks SET
			title=$2, description=$3, status=$4, priority=$5, assignee_id=$6, due_at=$7,
			progress=$8, weight=$9, blocked_by=$10, recurrence=$11, custom_fields=$12, updated_at=now()
		WHERE id=$1`,
		taskID, snap.Title, snap.Description, snap.Status, snap.Priority, snap.AssigneeID, snap.DueAt,
		snap.Progress, snap.Weight, snap.BlockedBy, snap.Recurrence, snap.CustomFields)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
