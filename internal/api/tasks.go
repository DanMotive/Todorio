package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/DanMotive/Todorio/internal/events"
)

type taskRow struct {
	ID           int64           `json:"id"`
	ListID       int64           `json:"list_id"`
	ParentID     *int64          `json:"parent_id"`
	Title        string          `json:"title"`
	Description  *string         `json:"description"`
	Status       string          `json:"status"`
	Priority     string          `json:"priority"`
	AssigneeID   *int64          `json:"assignee_id"`
	CreatorID    int64           `json:"creator_id"`
	StartAt      *time.Time      `json:"start_at"`
	DueAt        *time.Time      `json:"due_at"`
	Weight       int             `json:"weight"`
	Progress     *int            `json:"progress"`
	BlockedBy    []int64         `json:"blocked_by"`
	CustomFields json.RawMessage `json:"custom_fields"`
	Recurrence   json.RawMessage `json:"recurrence"`
	CompletedAt  *time.Time      `json:"completed_at"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	SubtaskDone  int             `json:"subtasks_done"`
	SubtaskAll   int             `json:"subtasks_total"`
	ActiveFocus  json.RawMessage `json:"active_focus"`
}

// active_focus is who (if anyone) currently has an open focus session on this task — the
// presence/"working on this right now" signal from spec, kept as a json_agg subquery so it rides
// along on every existing task read (list, my-tasks, single task) instead of needing N+1 calls.
const taskSelect = `
	SELECT t.id, t.list_id, t.parent_id, t.title, t.description, t.status, t.priority,
		t.assignee_id, t.creator_id, t.start_at, t.due_at, t.weight, t.progress,
		COALESCE(t.blocked_by, '{}'), t.custom_fields, t.recurrence,
		t.completed_at, t.created_at, t.updated_at,
		(SELECT count(*) FROM tasks s WHERE s.parent_id=t.id AND s.archived_at IS NULL AND s.completed_at IS NOT NULL)::int,
		(SELECT count(*) FROM tasks s WHERE s.parent_id=t.id AND s.archived_at IS NULL)::int,
		COALESCE((SELECT json_agg(json_build_object(
				'user_id', u.id, 'username', u.username, 'avatar_path', u.avatar_path, 'started_at', fs.started_at
			) ORDER BY fs.started_at)
			FROM focus_sessions fs JOIN users u ON u.id = fs.user_id
			WHERE fs.task_id = t.id AND fs.ended_at IS NULL), '[]')
	FROM tasks t`

func scanTask(row interface{ Scan(...any) error }) (taskRow, error) {
	var t taskRow
	err := row.Scan(&t.ID, &t.ListID, &t.ParentID, &t.Title, &t.Description, &t.Status, &t.Priority,
		&t.AssigneeID, &t.CreatorID, &t.StartAt, &t.DueAt, &t.Weight, &t.Progress,
		&t.BlockedBy, &t.CustomFields, &t.Recurrence,
		&t.CompletedAt, &t.CreatedAt, &t.UpdatedAt, &t.SubtaskDone, &t.SubtaskAll, &t.ActiveFocus)
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
	if limit := a.intSetting(r.Context(), "limits.content.tasks_per_list", 0); limit > 0 {
		var count int
		_ = a.DB.Pool.QueryRow(r.Context(),
			`SELECT count(*) FROM tasks WHERE list_id=$1 AND archived_at IS NULL`, listID).Scan(&count)
		if count >= limit {
			errJSON(w, http.StatusForbidden, "this list has reached its maximum number of tasks")
			return
		}
	}
	var in struct {
		Title       string     `json:"title"`
		Description *string    `json:"description"`
		Priority    *string    `json:"priority"`
		ParentID    *int64     `json:"parent_id"`
		AssigneeID  *int64     `json:"assignee_id"`
		StartAt     *time.Time `json:"start_at"`
		DueAt       *time.Time `json:"due_at"`
		Weight      *int       `json:"weight"`
		// Parse turns on smart quick-add: "#tag !priority @user tomorrow" is extracted from the
		// title (spec section 5). Opt-in per request so a plain create can't have its title
		// rewritten unexpectedly — e.g. a title that legitimately contains "#1".
		Parse bool `json:"parse"`
	}
	if err := readJSON(r, &in); err != nil || in.Title == "" {
		errJSON(w, http.StatusBadRequest, "a task title is required")
		return
	}
	if in.StartAt != nil && in.DueAt != nil && in.StartAt.After(*in.DueAt) {
		errJSON(w, http.StatusBadRequest, "the start date cannot be after the deadline")
		return
	}

	// Smart quick-add. Explicit fields always win over parsed ones: if the client sent an
	// assignee or a deadline outright, that was a deliberate choice and the text is only used
	// to fill what's still empty.
	var parsedTags []string
	if in.Parse {
		p := parseQuickAdd(in.Title, a.resolveUserForList(r.Context(), listID))
		if p.Title == "" {
			errJSON(w, http.StatusBadRequest, "a task title is required")
			return
		}
		in.Title = p.Title
		parsedTags = p.Tags
		if in.Priority == nil && p.Priority != "" {
			in.Priority = &p.Priority
		}
		if in.AssigneeID == nil && p.AssigneeID != nil {
			in.AssigneeID = p.AssigneeID
		}
		if in.DueAt == nil && p.DueAt != nil {
			in.DueAt = p.DueAt
		}
	}
	// description is NOT NULL in the schema: a client that omits the field (the ListView
	// quick-add form sends only title + due_at) decodes to a nil *string, which pgx binds
	// as SQL NULL and the insert fails with a not-null violation. Normalise to "" here,
	// the same way notes.go does for its NOT NULL body column.
	description := ""
	if in.Description != nil {
		description = *in.Description
	}
	var id int64
	err = a.DB.Pool.QueryRow(r.Context(), `
		INSERT INTO tasks(list_id, parent_id, title, description, priority, assignee_id, start_at, due_at, weight, creator_id)
		VALUES($1,$2,$3,$4,COALESCE($5,'normal'),$6,$7,$8,COALESCE($9,1),$10) RETURNING id`,
		listID, in.ParentID, in.Title, description, in.Priority, in.AssigneeID, in.StartAt, in.DueAt, in.Weight, u.ID).Scan(&id)
	if err != nil {
		dbFail(r, "create task", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	// Parsed #tags land in the "labels" multiselect custom field — the product has no separate
	// label system by design (see fields.go), so this is the one place they belong.
	if len(parsedTags) > 0 {
		labels, _ := json.Marshal(map[string]string{"labels": strings.Join(parsedTags, ",")})
		if _, err := a.DB.Pool.Exec(r.Context(),
			`UPDATE tasks SET custom_fields = custom_fields || $2::jsonb WHERE id=$1`,
			id, string(labels)); err != nil {
			// The task itself was created; failing to attach labels shouldn't 500 the request.
			dbFail(r, "attach quick-add labels", err)
		}
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
	var oldStatus, title string
	var oldDueAt *time.Time
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT list_id, assignee_id, status, due_at, title FROM tasks WHERE id=$1 AND archived_at IS NULL`, id).
		Scan(&listID, &oldAssignee, &oldStatus, &oldDueAt, &title) != nil {
		errJSON(w, http.StatusNotFound, "task not found")
		return
	}
	if !permAtLeast(a.listPermission(r, u, listID), "editor") {
		errJSON(w, http.StatusForbidden, "no permission to edit")
		return
	}
	var in struct {
		Title         *string `json:"title"`
		Description   *string `json:"description"`
		Status        *string `json:"status"`
		Priority      *string `json:"priority"`
		AssigneeID    *int64  `json:"assignee_id"`
		ClearAssignee bool    `json:"clear_assignee"`
		// StartAt is the Timeline/Gantt bar's left edge (spec section 12). Like due_at it needs
		// an explicit clear flag, since COALESCE can't tell "null" from "field omitted".
		StartAt      *time.Time `json:"start_at"`
		ClearStartAt bool       `json:"clear_start_at"`
		DueAt        *time.Time `json:"due_at"`
		ClearDueAt   bool       `json:"clear_due_at"`
		Progress     *int       `json:"progress"`
		// ClearProgress removes a manual progress override so the task falls back to automatic
		// progress from its subtasks. A plain "progress": null can't express this — it's
		// indistinguishable from omitting the field once COALESCE is applied below.
		ClearProgress   bool               `json:"clear_progress"`
		Weight          *int               `json:"weight"`
		BlockedBy       *[]int64           `json:"blocked_by"`
		Position        *int               `json:"position"`
		Recurrence      *recurrenceRule    `json:"recurrence"`
		ClearRecurrence bool               `json:"clear_recurrence"`
		CustomFields    *map[string]string `json:"custom_fields"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	// progress is a SMALLINT percentage and weight feeds weighted progress/ranking maths —
	// clamp both here rather than letting a malformed client write nonsense that every
	// consumer would then have to defend against.
	if in.Progress != nil && (*in.Progress < 0 || *in.Progress > 100) {
		errJSON(w, http.StatusBadRequest, "progress must be between 0 and 100")
		return
	}
	if in.Weight != nil && (*in.Weight < 1 || *in.Weight > 100) {
		errJSON(w, http.StatusBadRequest, "weight must be between 1 and 100")
		return
	}
	// A start after the deadline would draw a backwards Timeline bar. Validate against the
	// value the task will actually end up with, not just what's in this request: either side
	// of the range can be unchanged, cleared, or set here.
	if newStart, newDue := in.StartAt, in.DueAt; newStart != nil || newDue != nil {
		if newStart == nil && !in.ClearStartAt {
			_ = a.DB.Pool.QueryRow(r.Context(), `SELECT start_at FROM tasks WHERE id=$1`, id).Scan(&newStart)
		}
		if newDue == nil && !in.ClearDueAt {
			newDue = oldDueAt
		}
		if newStart != nil && newDue != nil && newStart.After(*newDue) {
			errJSON(w, http.StatusBadRequest, "the start date cannot be after the deadline")
			return
		}
	}
	if in.Status != nil {
		allowed := a.listStatuses(r, listID)
		ok := false
		for _, s := range allowed {
			if s == *in.Status {
				ok = true
				break
			}
		}
		if !ok {
			errJSON(w, http.StatusBadRequest, "unknown status for this space's workflow")
			return
		}
	}

	// snapshot of the version before the change (unless versioning is disabled instance-wide)
	if a.featureEnabled(r.Context(), "versions") {
		_, _ = a.DB.Pool.Exec(r.Context(), `
			INSERT INTO task_versions(task_id, editor_id, snapshot)
			SELECT id, $2, to_jsonb(t) FROM tasks t WHERE id=$1`, id, u.ID)
	}

	_, err = a.DB.Pool.Exec(r.Context(), `
		UPDATE tasks SET
			title       = COALESCE($2, title),
			description = COALESCE($3, description),
			status      = COALESCE($4, status),
			priority    = COALESCE($5, priority),
			assignee_id = CASE WHEN $7 THEN NULL ELSE COALESCE($6, assignee_id) END,
			due_at      = CASE WHEN $9 THEN NULL ELSE COALESCE($8, due_at) END,
			start_at    = CASE WHEN $19 THEN NULL ELSE COALESCE($18, start_at) END,
			progress    = CASE WHEN $17 THEN NULL ELSE COALESCE($10, progress) END,
			weight      = COALESCE($11, weight),
			blocked_by  = COALESCE($12, blocked_by),
			position    = COALESCE($13, position),
			recurrence  = CASE WHEN $14 THEN NULL ELSE COALESCE($15, recurrence) END,
			custom_fields = COALESCE($16, custom_fields),
			completed_at = CASE
				WHEN $4 = 'done' AND completed_at IS NULL THEN now()
				WHEN $4 IS NOT NULL AND $4 <> 'done' THEN NULL
				ELSE completed_at END,
			updated_at  = now()
		WHERE id=$1`,
		id, in.Title, in.Description, in.Status, in.Priority,
		in.AssigneeID, in.ClearAssignee, in.DueAt, in.ClearDueAt,
		in.Progress, in.Weight, in.BlockedBy, in.Position,
		in.ClearRecurrence, in.Recurrence, in.CustomFields, in.ClearProgress,
		in.StartAt, in.ClearStartAt)
	if err != nil {
		dbFail(r, "update task", err)
		errJSON(w, http.StatusBadRequest, "update failed (check the values)")
		return
	}

	// recurring tasks: spawn the next occurrence once this one is marked done
	if in.Status != nil && *in.Status == "done" {
		a.spawnRecurrence(r.Context(), id)
	}

	if in.AssigneeID != nil && (oldAssignee == nil || *oldAssignee != *in.AssigneeID) && *in.AssigneeID != u.ID {
		a.notify(r, *in.AssigneeID, "task_assigned", map[string]any{"task_id": id, "title": title, "by": u.Username})
	}

	// "изменение дедлайна/статуса" (spec section 7): notify whoever is assigned after this update,
	// as long as it isn't the person making the change themselves.
	notifyAssignee := oldAssignee
	if in.ClearAssignee {
		notifyAssignee = nil
	} else if in.AssigneeID != nil {
		notifyAssignee = in.AssigneeID
	}
	statusChanged := in.Status != nil && *in.Status != oldStatus
	dueChanged := (in.ClearDueAt && oldDueAt != nil) || (in.DueAt != nil && (oldDueAt == nil || !in.DueAt.Equal(*oldDueAt)))
	assigneeChanged := (in.ClearAssignee && oldAssignee != nil) || (in.AssigneeID != nil && (oldAssignee == nil || *oldAssignee != *in.AssigneeID))
	if notifyAssignee != nil && *notifyAssignee != u.ID {
		if statusChanged {
			a.notify(r, *notifyAssignee, "status_changed", map[string]any{"task_id": id, "title": title, "status": *in.Status, "by": u.Username})
		}
		if dueChanged {
			a.notify(r, *notifyAssignee, "due_changed", map[string]any{"task_id": id, "title": title, "by": u.Username})
		}
	}
	// Same three changes also land as system records in the task's own comment feed (spec section
	// 7: "системные записи в ленте"), visible to everyone looking at the task, independent of who
	// is or isn't assigned/notified above. Body is small structured JSON, not a frozen sentence in
	// whichever language the editor happened to be using — the frontend formats it with tr().
	if statusChanged {
		a.insertSystemComment(r, id, u.ID, "status_changed", map[string]any{"from": oldStatus, "to": *in.Status})
	}
	if dueChanged {
		a.insertSystemComment(r, id, u.ID, "due_changed", nil)
	}
	if assigneeChanged {
		a.insertSystemComment(r, id, u.ID, "assignee_changed", nil)
	}
	a.publishToListMembers(r, listID, events.Event{Type: "task.updated", Data: map[string]any{"task_id": id, "list_id": listID}})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// insertSystemComment records a system event (status/due/assignee change, ...) as an is_system
// comment row, so it shows up inline in the task's own comment feed for everyone looking at it —
// distinct from notify(), which pings one specific recipient.
func (a *API) insertSystemComment(r *http.Request, taskID, authorID int64, kind string, extra map[string]any) {
	body := map[string]any{"type": kind}
	for k, v := range extra {
		body[k] = v
	}
	b, _ := json.Marshal(body)
	_, _ = a.DB.Pool.Exec(r.Context(),
		`INSERT INTO comments(task_id, author_id, body, is_system) VALUES($1,$2,$3,true)`, taskID, authorID, string(b))
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
	_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE tasks SET archived_at=now(), archived_by=$2 WHERE id=$1 OR parent_id=$1`, id, u.ID)
	a.publishToListMembers(r, listID, events.Event{Type: "task.archived", Data: map[string]any{"task_id": id, "list_id": listID}})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/tasks/{id}/restore — undoes an archive, resetting the 30-day auto-cleanup countdown
// (cleanupArchive only ever looks at archived_at, so clearing it removes the task from
// consideration entirely). Also restores any subtasks archived in the same action (mirrors the
// OR parent_id=$1 cascade in handleArchiveTask above). Same permission as archiving.
func (a *API) handleRestoreTask(w http.ResponseWriter, r *http.Request) {
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
	_, _ = a.DB.Pool.Exec(r.Context(),
		`UPDATE tasks SET archived_at=NULL, archived_by=NULL WHERE (id=$1 OR parent_id=$1) AND archived_at IS NOT NULL`, id)
	a.publishToListMembers(r, listID, events.Event{Type: "task.restored", Data: map[string]any{"task_id": id, "list_id": listID}})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/tasks/{id}/permanent — irreversible, root only, and only for tasks already archived
// (archive-then-purge, matching the same two-step safety net as lists/spaces below).
func (a *API) handleDeleteTaskPermanent(w http.ResponseWriter, r *http.Request) {
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
	tag, err := a.DB.Pool.Exec(r.Context(), `DELETE FROM tasks WHERE id=$1 AND archived_at IS NOT NULL`, id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if tag.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "task not found or not archived — archive it first")
		return
	}
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
