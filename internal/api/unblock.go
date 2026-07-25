package api

// Dependency unblocking.
//
// tasks.blocked_by has existed since migration 0002 and is drawn on the timeline, but nothing
// ever reacted to it: when the blocking task was finally finished, whoever was waiting on it
// found out by chance, next time they happened to open the task. The dependency was recorded
// and then ignored, which is the worst of both worlds - the data implies the app is tracking
// something it is not.
//
// This closes the loop. Completing a task looks for everything that was waiting on it and, for
// those that now have no unfinished blockers left, notifies the assignee and the watchers and
// drops a system record in the task's feed.

import (
	"net/http"

	"github.com/DanMotive/Todorio/internal/auth"
	"github.com/DanMotive/Todorio/internal/events"
)

// unblockedTask is one task that just became workable.
type unblockedTask struct {
	ID       int64
	ListID   int64
	Title    string
	Assignee *int64
}

// notifyUnblocked is called after a task has been marked done. It is deliberately best-effort:
// a failure here must never turn a successful "mark as done" into an error response, because the
// completion itself is already committed by the time we get here.
//
// Only fully-unblocked tasks are announced. A task waiting on three things does not want a ping
// each time one of them lands - that is noise, not news; the useful moment is the one where it
// can actually be started.
func (a *API) notifyUnblocked(r *http.Request, finishedID int64, actor *auth.User) {
	for _, t := range a.findUnblocked(r, finishedID) {
		// The assignee is the person actually waiting on this. Skipped when they are the one who
		// closed the blocker: they already know, they just did it.
		if t.Assignee != nil && *t.Assignee != actor.ID {
			a.notify(r, *t.Assignee, "task_unblocked", map[string]any{
				"task_id": t.ID, "title": t.Title, "by": actor.Username,
			})
		}
		// notifyWatchers already skips the actor and the assignee, so nobody is told twice.
		a.notifyWatchers(r, t.ID, actor.ID, "task_unblocked", map[string]any{
			"task_id": t.ID, "title": t.Title, "by": actor.Username,
		})
		// A system record in the feed, so the reason the task became workable is visible later to
		// anyone reading its history - including people who were never notified.
		a.insertSystemComment(r, t.ID, actor.ID, "unblocked", map[string]any{"by_task": finishedID})
		a.publishToListMembers(r, t.ListID, events.Event{
			Type: "task.updated",
			Data: map[string]any{"task_id": t.ID, "list_id": t.ListID},
		})
	}
}

// findUnblocked returns the tasks that list finishedID as a blocker and have no unfinished
// blockers left at all.
//
// The NOT EXISTS subquery re-checks every blocker rather than trusting that finishedID was the
// only one, so the result is correct regardless of how many dependencies a task has or in what
// order they were completed.
//
// A blocker id that no longer resolves to a row (the blocking task was purged) counts as
// satisfied: blocked_by is a plain BIGINT[] with no foreign key, so deleting a task leaves a
// dangling id behind. Treating that as "still blocking" would strand the dependent task forever
// with no way for a user to discover why.
func (a *API) findUnblocked(r *http.Request, finishedID int64) []unblockedTask {
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT t.id, t.list_id, t.title, t.assignee_id
		FROM tasks t
		WHERE $1 = ANY(t.blocked_by)
		  AND t.completed_at IS NULL
		  AND t.archived_at IS NULL
		  AND NOT EXISTS (
			  SELECT 1 FROM tasks b
			  WHERE b.id = ANY(t.blocked_by)
			    AND b.completed_at IS NULL
			    AND b.archived_at IS NULL
		  )`, finishedID)
	if err != nil {
		dbFail(r, "find unblocked tasks", err)
		return nil
	}
	defer rows.Close()

	var out []unblockedTask
	for rows.Next() {
		var t unblockedTask
		if rows.Scan(&t.ID, &t.ListID, &t.Title, &t.Assignee) == nil {
			out = append(out, t)
		}
	}
	return out
}
