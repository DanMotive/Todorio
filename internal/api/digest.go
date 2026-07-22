package api

import (
	"net/http"
	"time"
)

// GET /api/digest — "while you were away" summary.
// prev_seen_at is set by middleware on return after a pause of ≥6 hours
// and cleared after viewing — the digest "doesn't pop up when it shouldn't".
func (a *API) handleDigest(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var since *time.Time
	if a.DB.Pool.QueryRow(r.Context(), `SELECT prev_seen_at FROM users WHERE id=$1`, u.ID).Scan(&since) != nil ||
		since == nil {
		writeJSON(w, http.StatusOK, map[string]any{"show": false})
		return
	}

	var assigned, comments, doneAround, announcements int
	_ = a.DB.Pool.QueryRow(r.Context(), `
		SELECT
			(SELECT count(*) FROM tasks WHERE assignee_id=$1 AND archived_at IS NULL AND created_at > $2),
			(SELECT count(*) FROM comments c JOIN tasks t ON t.id=c.task_id
				WHERE c.created_at > $2 AND c.author_id <> $1 AND c.deleted_at IS NULL
				  AND (t.assignee_id=$1 OR t.creator_id=$1)),
			(SELECT count(*) FROM tasks WHERE completed_at > $2 AND archived_at IS NULL
				AND list_id IN (SELECT list_id FROM list_members WHERE user_id=$1)),
			(SELECT count(*) FROM announcements WHERE created_at > $2
				AND (space_id IS NULL OR space_id IN (SELECT space_id FROM space_members WHERE user_id=$1)))`,
		u.ID, *since).Scan(&assigned, &comments, &doneAround, &announcements)

	if assigned+comments+doneAround+announcements == 0 {
		// nothing happened — don't bother the user, and clear the flag
		_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE users SET prev_seen_at=NULL WHERE id=$1`, u.ID)
		writeJSON(w, http.StatusOK, map[string]any{"show": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"show":  true,
		"since": since,
		"summary": map[string]int{
			"assigned_to_me": assigned,
			"new_comments":   comments,
			"done_nearby":    doneAround,
			"announcements":  announcements,
		},
	})
}

// POST /api/digest/dismiss — dismiss the summary until the next long pause.
func (a *API) handleDigestDismiss(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE users SET prev_seen_at=NULL WHERE id=$1`, u.ID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
