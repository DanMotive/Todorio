package api

import (
	"net/http"
	"time"

	"github.com/DanMotive/Todorio/internal/events"
)

type taskRow struct {
	ID          int64      `json:"id"`
	ListID      int64      `json:"list_id"`
	ParentID    *int64     `json:"parent_id"`
	Title       string     `json:"title"`
	Description *string    `json:"description"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	AssigneeID  *int64     `json:"assignee_id"`
	CreatorID   int64      `json:"creator_id"`
	DueAt       *time.Time `json:"due_at"`
	Weight      int        `json:"weight"`
	Progress    *int       `json:"progress"`
	BlockedBy   []int64    `json:"blocked_by"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	SubtaskDone int        `json:"subtasks_done"`
	SubtaskAll  int        `json:"subtasks_total"`
}

const taskSelect = `
	SELECT t.id, t.list_id, t.parent_id, t.title, t.description, t.status, t.priority,
		t.assignee_id, t.creator_id, t.due_at, t.weight, t.progress,
		COALESCE(t.blocked_by, '{}'), t.completed_at, t.created_at, t.updated_at,
		(SELECT count(*) FROM tasks s WHERE s.parent_id=t.id AND s.archived_at IS NULL AND s.completed_at IS NOT NULL)::int,
		(SELECT count(*) FROM tasks s WHERE s.parent_id=t.id AND s.archived_at IS NULL)::int
	FROM tasks t`

func scanTask(row interface{ Scan(...any) error }) (taskRow, error) {
	var t taskRow
	err := row.Scan(&t.ID, &t.ListID, &t.ParentID, &t.Title, &t.Description, &t.Status, &t.Priority,
		&t.AssigneeID, &t.CreatorID, &t.DueAt, &t.Weight, &t.Progress,
		&t.BlockedBy, &t.CompletedAt, &t.CreatedAt, &t.UpdatedAt, &t.SubtaskDone, &t.SubtaskAll)
	return t, err
}

// GET /api/lists/{id}/tasks
func (a *API) handleListTasks(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	listID, err := pathID(r)
	if err != nil || !permAtLeast(a.listPermission(r, u, listID), "viewer") {
		errJSON(w, http.StatusForbidden, "no access to the list")
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(),
		taskSelect+` WHERE t.list_id=$1 AND t.archived_at IS NULL ORDER BY t.position, t.id`, listID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	tasks := []taskRow{}
	for rows.Next() {
		if t, err := scanTask(rows); err == nil {
			tasks = append(tasks, t)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

// GET /api/my/tasks — the "My tasks" screen: all open tasks assigned to me, nearest deadlines first.
func (a *API) handleMyTasks(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(),
		taskSelect+` WHERE t.assignee_id=$1 AND t.archived_at IS NULL AND t.completed_at IS NULL
		ORDER BY t.due_at NULLS LAST, t.priority DESC, t.id`, u.ID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	tasks := []taskRow{}
	for rows.Next() {
		if t, err := scanTask(rows); err == nil {
			tasks = append(tasks, t)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (a *API) handleGetTask(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	t, err := scanTask(a.DB.Pool.QueryRow(r.Context(), taskSelect+` WHERE t.id=$1 AND t.archived_at IS NULL`, id))
	if err != nil {
		errJSON(w, http.StatusNotFound, "task not found")
		return
	}
	if !permAtLeast(a.listPermission(r, u, t.ListID), "viewer") {
		errJSON(w, http.StatusForbidden, "no access")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": t})
}

// POST /api/lists/{id}/tasks
func (a *API) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	listID, err := pathID(r)
	if err != nil || !permAtLeast(a.listPermission(r, u, listID), "editor") {
		errJSON(w, http.StatusForbidden, "no permission to create tasks")
		return
	}
	var in struct {
		Title       string     `json:"title"`
		Description *string    `json:"description"`
		Priority    *string    `json:"priority"`
		ParentID    *int64     `json:"parent_id"`
		AssigneeID  *int64     `json:"assignee_id"`
		DueAt       *time.Time `json:"due_at"`
		Weight      *int       `json:"weight"`
	}
	if err := readJSON(r, &in); err != nil || in.Title == "" {
		errJSON(w, http.StatusBadRequest, "a task title is required")
		return
	}
	var id int64
	err = a.DB.Pool.QueryRow(r.Context(), `
		INSERT INTO tasks(list_id, parent_id, title, description, priority, assignee_id, due_at, weight, creator_id)
		VALUES($1,$2,$3,$4,COALESCE($5,'normal'),$6,$7,COALESCE($8,1),$9) RETURNING id`,
		listID, in.ParentID, in.Title, in.Description, in.Priority, in.AssigneeID, in.DueAt, in.Weight, u.ID).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if in.AssigneeID != nil && *in.AssigneeID != u.ID {
		a.notify(r, *in.AssigneeID, "task_assigned", map[string]any{"task_id": id, "title": in.Title, "by": u.Username})
	}
	a.publishToListMembers(r, listID, events.Event{Type: "task.created", Data: map[string]any{"task_id": id, "list_id": listID}})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// PATCH /api/tasks/{id} — any field; a snapshot is written to task_versions before the change.
func (a *API) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var listID int64
	var oldAssignee *int64
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT list_id, assignee_id FROM tasks WHERE id=$1 AND archived_at IS NULL`, id).Scan(&listID, &oldAssignee) != nil {
		errJSON(w, http.StatusNotFound, "task not found")
		return
	}
	if !permAtLeast(a.listPermission(r, u, listID), "editor") {
		errJSON(w, http.StatusForbidden, "no permission to edit")
		return
	}
	var in struct {
		Title       *string    `json:"title"`
		Description *string    `json:"description"`
		Status      *string    `json:"status"`
		Priority    *string    `json:"priority"`
		AssigneeID  *int64     `json:"assignee_id"`
		ClearAssignee bool     `json:"clear_assignee"`
		DueAt       *time.Time `json:"due_at"`
		ClearDueAt  bool       `json:"clear_due_at"`
		Progress    *int       `json:"progress"`
		Weight      *int       `json:"weight"`
		BlockedBy   *[]int64   `json:"blocked_by"`
		Position    *int       `json:"position"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}

	// snapshot of the version before the change
	_, _ = a.DB.Pool.Exec(r.Context(), `
		INSERT INTO task_versions(task_id, editor_id, snapshot)
		SELECT id, $2, to_jsonb(t) FROM tasks t WHERE id=$1`, id, u.ID)

	_, err = a.DB.Pool.Exec(r.Context(), `
		UPDATE tasks SET
			title       = COALESCE($2, title),
			description = COALESCE($3, description),
			status      = COALESCE($4, status),
			priority    = COALESCE($5, priority),
			assignee_id = CASE WHEN $7 THEN NULL ELSE COALESCE($6, assignee_id) END,
			due_at      = CASE WHEN $9 THEN NULL ELSE COALESCE($8, due_at) END,
			progress    = COALESCE($10, progress),
			weight      = COALESCE($11, weight),
			blocked_by  = COALESCE($12, blocked_by),
			position    = COALESCE($13, position),
			completed_at = CASE
				WHEN $4 = 'done' AND completed_at IS NULL THEN now()
				WHEN $4 IS NOT NULL AND $4 <> 'done' THEN NULL
				ELSE completed_at END,
			updated_at  = now()
		WHERE id=$1`,
		id, in.Title, in.Description, in.Status, in.Priority,
		in.AssigneeID, in.ClearAssignee, in.DueAt, in.ClearDueAt,
		in.Progress, in.Weight, in.BlockedBy, in.Position)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "update failed (check the values)")
		return
	}

	// recurring tasks: spawn the next occurrence once this one is marked done
	if in.Status != nil && *in.Status == "done" {
		a.spawnRecurrence(r.Context(), id)
	}

	if in.AssigneeID != nil && (oldAssignee == nil || *oldAssignee != *in.AssigneeID) && *in.AssigneeID != u.ID {
		a.notify(r, *in.AssigneeID, "task_assigned", map[string]any{"task_id": id, "by": u.Username})
	}
	a.publishToListMembers(r, listID, events.Event{Type: "task.updated", Data: map[string]any{"task_id": id, "list_id": listID}})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/tasks/{id} — moves to archive.
func (a *API) handleArchiveTask(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var listID int64
	if a.DB.Pool.QueryRow(r.Context(), `SELECT list_id FROM tasks WHERE id=$1`, id).Scan(&listID) != nil {
		errJSON(w, http.StatusNotFound, "task not found")
		return
	}
	if !permAtLeast(a.listPermission(r, u, listID), "editor") {
		errJSON(w, http.StatusForbidden, "no permission")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE tasks SET archived_at=now() WHERE id=$1 OR parent_id=$1`, id)
	a.publishToListMembers(r, listID, events.Event{Type: "task.archived", Data: map[string]any{"task_id": id, "list_id": listID}})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// publishToListMembers broadcasts an SSE event to every member of the list.
func (a *API) publishToListMembers(r *http.Request, listID int64, e events.Event) {
	rows, err := a.DB.Pool.Query(r.Context(), `SELECT user_id FROM list_members WHERE list_id=$1`, listID)
	if err != nil {
		return
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	a.Bus.Publish(ids, e)
}
