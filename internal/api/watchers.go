package api

// Watchers and the review workflow (spec section 5).
//
// Watchers: a task field the spec lists but that never existed. A watcher follows a task without
// owning it and receives the same notifications the assignee gets. Being a watcher grants no
// extra access — you can only watch a task you can already see, so this cannot be used to
// subscribe your way into a private list.
//
// Review mode: "на проверке" used to be nothing but a status string. It now carries a real
// decision — who submitted, who accepted or returned it, when, and why — because "returned"
// without a reason tells the author nothing about what to fix.

import (
	"net/http"
	"time"

	"github.com/DanMotive/Todorio/internal/auth"
)

// POST /api/tasks/{id}/watch — start watching. Idempotent (the table's composite PK absorbs
// repeats), so a double click is harmless.
func (a *API) handleWatchTask(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	taskID, listID, ok := a.taskWithListAccess(w, r, u, "viewer")
	if !ok {
		return
	}
	_ = listID
	if _, err := a.DB.Pool.Exec(r.Context(),
		`INSERT INTO task_watchers(task_id, user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`,
		taskID, u.ID); err != nil {
		dbFail(r, "watch task", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"watching": true})
}

// DELETE /api/tasks/{id}/watch — stop watching.
func (a *API) handleUnwatchTask(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	taskID, _, ok := a.taskWithListAccess(w, r, u, "viewer")
	if !ok {
		return
	}
	if _, err := a.DB.Pool.Exec(r.Context(),
		`DELETE FROM task_watchers WHERE task_id=$1 AND user_id=$2`, taskID, u.ID); err != nil {
		dbFail(r, "unwatch task", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"watching": false})
}

// GET /api/tasks/{id}/watchers — who is watching, and whether the caller is among them.
func (a *API) handleListWatchers(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	taskID, _, ok := a.taskWithListAccess(w, r, u, "viewer")
	if !ok {
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT u.id, u.username, COALESCE(u.display_name, u.username)
		FROM task_watchers tw JOIN users u ON u.id = tw.user_id
		WHERE tw.task_id = $1 AND u.archived_at IS NULL
		ORDER BY tw.created_at`, taskID)
	if err != nil {
		dbFail(r, "list watchers", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	type watcher struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	}
	list := []watcher{}
	mine := false
	for rows.Next() {
		var wt watcher
		if rows.Scan(&wt.ID, &wt.Username, &wt.Name) == nil {
			list = append(list, wt)
			if wt.ID == u.ID {
				mine = true
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"watchers": list, "watching": mine})
}

// notifyWatchers sends a notification to every watcher of a task, skipping the person who caused
// the event (nobody needs telling about their own action) and the assignee, who is notified
// through the existing per-event paths and would otherwise get the same thing twice.
func (a *API) notifyWatchers(r *http.Request, taskID int64, actorID int64, kind string, data map[string]any) {
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT tw.user_id FROM task_watchers tw
		JOIN tasks t ON t.id = tw.task_id
		WHERE tw.task_id = $1
		  AND tw.user_id <> $2
		  AND COALESCE(t.assignee_id, 0) <> tw.user_id`, taskID, actorID)
	if err != nil {
		dbFail(r, "notify watchers", err)
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
	for _, id := range ids {
		a.notify(r, id, kind, data)
	}
}

// --- review workflow ---

// POST /api/tasks/{id}/review/submit — hand a task over for review.
//
// Anyone who can edit the task may submit it: in a small team the person doing the work is
// usually the one who decides it's ready.
func (a *API) handleSubmitReview(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	taskID, listID, ok := a.taskWithListAccess(w, r, u, "editor")
	if !ok {
		return
	}
	if _, err := a.DB.Pool.Exec(r.Context(), `
		UPDATE tasks SET status='review', review_state='pending', review_by=NULL,
			review_at=now(), review_note=NULL, updated_at=now()
		WHERE id=$1`, taskID); err != nil {
		dbFail(r, "submit review", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	a.addSystemComment(r, taskID, u, "review_submitted", "")
	// The list owner is the one who decides, so they're the one who needs to know.
	a.notifyListOwners(r, listID, taskID, u.ID, "review_requested")
	a.notifyWatchers(r, taskID, u.ID, "review_requested", map[string]any{"task_id": taskID, "by": u.Username})
	writeJSON(w, http.StatusOK, map[string]string{"review_state": "pending"})
}

// POST /api/tasks/{id}/review/decide {accept, note?} — accept or return a task under review.
//
// Requires owner permission on the list: the spec is explicit that the owner accepts or returns.
// Accepting marks the task done; returning puts it back to in_progress with the reviewer's note,
// so the author sees both that it came back and why.
func (a *API) handleDecideReview(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	taskID, _, ok := a.taskWithListAccess(w, r, u, "owner")
	if !ok {
		return
	}
	var in struct {
		Accept bool   `json:"accept"`
		Note   string `json:"note"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	// A return with no explanation is useless to the author — require one.
	if !in.Accept && in.Note == "" {
		errJSON(w, http.StatusBadRequest, "a reason is required when returning a task")
		return
	}

	var authorID *int64
	_ = a.DB.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(assignee_id, creator_id) FROM tasks WHERE id=$1`, taskID).Scan(&authorID)

	state, status := "returned", "in_progress"
	if in.Accept {
		state, status = "accepted", "done"
	}
	if _, err := a.DB.Pool.Exec(r.Context(), `
		UPDATE tasks SET
			review_state=$2, review_by=$3, review_at=now(), review_note=NULLIF($4,''),
			status=$5,
			completed_at = CASE WHEN $2='accepted' THEN COALESCE(completed_at, now()) ELSE NULL END,
			updated_at=now()
		WHERE id=$1`, taskID, state, u.ID, in.Note, status); err != nil {
		dbFail(r, "decide review", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}

	kind := "review_returned"
	if in.Accept {
		kind = "review_accepted"
		// An accepted task is finished work: stop any focus timer still running on it, the same
		// as any other completion path.
		a.closeFocusForTask(r, taskID, u)
	}
	a.addSystemComment(r, taskID, u, kind, in.Note)
	if authorID != nil && *authorID != u.ID {
		a.notify(r, *authorID, kind, map[string]any{"task_id": taskID, "by": u.Username, "note": in.Note})
	}
	a.notifyWatchers(r, taskID, u.ID, kind, map[string]any{"task_id": taskID, "by": u.Username})

	writeJSON(w, http.StatusOK, map[string]any{"review_state": state, "status": status})
}

// notifyListOwners tells everyone with owner permission on a list about a task event.
func (a *API) notifyListOwners(r *http.Request, listID, taskID, actorID int64, kind string) {
	rows, err := a.DB.Pool.Query(r.Context(),
		`SELECT user_id FROM list_members WHERE list_id=$1 AND permission='owner' AND user_id<>$2`,
		listID, actorID)
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
	for _, id := range ids {
		a.notify(r, id, kind, map[string]any{"task_id": taskID})
	}
}

// taskWithListAccess resolves a task id from the path, checks the caller has at least `need`
// permission on its list, and writes the error response itself when they don't. Returns
// (taskID, listID, ok) so callers stay short.
func (a *API) taskWithListAccess(w http.ResponseWriter, r *http.Request, u *auth.User, need string) (int64, int64, bool) {
	taskID, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return 0, 0, false
	}
	var listID int64
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT list_id FROM tasks WHERE id=$1 AND archived_at IS NULL`, taskID).Scan(&listID) != nil {
		errJSON(w, http.StatusNotFound, "task not found")
		return 0, 0, false
	}
	if !permAtLeast(a.listPermission(r, u, listID), need) {
		errJSON(w, http.StatusForbidden, "no permission")
		return 0, 0, false
	}
	return taskID, listID, true
}

// addSystemComment records a review decision in the task's discussion feed, so the history is
// visible where people actually look rather than only in a notification that scrolls away.
func (a *API) addSystemComment(r *http.Request, taskID int64, u *auth.User, kind, note string) {
	body := "review:" + kind
	if note != "" {
		body += " " + note
	}
	_, _ = a.DB.Pool.Exec(r.Context(),
		`INSERT INTO comments(task_id, author_id, body, is_system) VALUES($1,$2,$3,TRUE)`,
		taskID, u.ID, body)
}

// reviewInfo is the review block returned with a task.
type reviewInfo struct {
	State *string    `json:"state"`
	By    *string    `json:"by"`
	At    *time.Time `json:"at"`
	Note  *string    `json:"note"`
}
