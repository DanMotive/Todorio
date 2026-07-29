package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/DanMotive/Todorio/internal/events"
)

// GET /api/tasks/{id}/comments
func (a *API) handleListComments(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
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
		SELECT c.id, c.author_id, u.username, c.body, c.created_at, c.edited_at, c.is_system,
			COALESCE((SELECT json_agg(json_build_object('emoji', rx.emoji, 'user_id', rx.user_id))
				FROM reactions rx WHERE rx.target_type='comment' AND rx.target_id=c.id), '[]'::json),
			c.parent_id
		FROM comments c JOIN users u ON u.id=c.author_id
		WHERE c.task_id=$1 AND c.deleted_at IS NULL
		-- Group replies under their thread root, then chronologically within the thread.
		ORDER BY COALESCE(c.parent_id, c.id), c.parent_id NULLS FIRST, c.created_at`, taskID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	comments := []map[string]any{}
	for rows.Next() {
		var id, authorID int64
		var username, body string
		var createdAt, reactions any
		var editedAt *time.Time
		var isSystem bool
		var parentID *int64
		if rows.Scan(&id, &authorID, &username, &body, &createdAt, &editedAt, &isSystem, &reactions, &parentID) == nil {
			comments = append(comments, map[string]any{
				"id": id, "author_id": authorID, "author": username, "body": body,
				"created_at": createdAt, "edited_at": editedAt, "is_system": isSystem, "reactions": reactions,
				"parent_id": parentID,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": comments})
}

// POST /api/tasks/{id}/comments {body} — @mentions send notifications; the assignee and task author are notified too.
func (a *API) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if !a.featureEnabled(r.Context(), "comments") {
		errJSON(w, http.StatusForbidden, "comments are disabled on this server")
		return
	}
	taskID, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var listID, creatorID int64
	var assigneeID *int64
	var title string
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT list_id, creator_id, assignee_id, title FROM tasks WHERE id=$1 AND archived_at IS NULL`,
		taskID).Scan(&listID, &creatorID, &assigneeID, &title) != nil {
		errJSON(w, http.StatusNotFound, "task not found")
		return
	}
	if !permAtLeast(a.listPermission(r, u, listID), "viewer") {
		errJSON(w, http.StatusForbidden, "no access")
		return
	}
	if limit := a.intSetting(r.Context(), "limits.content.comments_per_task", 0); limit > 0 {
		var count int
		_ = a.DB.Pool.QueryRow(r.Context(),
			`SELECT count(*) FROM comments WHERE task_id=$1 AND deleted_at IS NULL`, taskID).Scan(&count)
		if count >= limit {
			errJSON(w, http.StatusForbidden, "this task has reached its maximum number of comments")
			return
		}
	}
	var in struct {
		Body string `json:"body"`
		// ParentID makes this a reply (migration 0009). Threads are one level deep on purpose:
		// a reply to a reply is flattened onto the same thread, which keeps the feed readable
		// instead of drifting indefinitely to the right.
		ParentID *int64 `json:"parent_id"`
	}
	if err := readJSON(r, &in); err != nil || in.Body == "" {
		errJSON(w, http.StatusBadRequest, "comment cannot be empty")
		return
	}
	// A parent must exist on THIS task — otherwise a crafted id could graft a reply onto a
	// conversation in a task the caller can't even see.
	var parentAuthor *int64
	if in.ParentID != nil {
		var pTask int64
		var pParent *int64
		var pAuthor int64
		if a.DB.Pool.QueryRow(r.Context(),
			`SELECT task_id, parent_id, author_id FROM comments WHERE id=$1 AND deleted_at IS NULL`,
			*in.ParentID).Scan(&pTask, &pParent, &pAuthor) != nil || pTask != taskID {
			errJSON(w, http.StatusBadRequest, "the comment being replied to was not found in this task")
			return
		}
		// Flatten: replying to a reply attaches to the original thread root.
		if pParent != nil {
			in.ParentID = pParent
		}
		parentAuthor = &pAuthor
	}
	maxLen := a.intSetting(r.Context(), "limits.content.comment_max_len", 4000)
	if maxLen > 0 && len(in.Body) > maxLen {
		errJSON(w, http.StatusBadRequest, fmt.Sprintf("comment is longer than %d characters", maxLen))
		return
	}
	var id int64
	if err := a.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO comments(task_id, author_id, body, parent_id) VALUES($1,$2,$3,$4) RETURNING id`,
		taskID, u.ID, in.Body, in.ParentID).Scan(&id); err != nil {
		dbFail(r, "create comment", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}

	// recipients: mentioned users + assignee + task author (deduplicated, excluding the comment's own author)
	recipients := map[int64]bool{creatorID: true}
	if assigneeID != nil {
		recipients[*assigneeID] = true
	}
	// Mentions resolve through mentions.go: a boundary before the `@` (so an email address in
	// the body is not a mention), and only users who can see this list — notifying someone
	// without access would hand them the task's title in the payload.
	for _, mid := range a.mentionedUserIDs(r.Context(), listID, in.Body) {
		recipients[mid] = true
	}
	// The person being replied to is the most directly concerned party.
	if parentAuthor != nil {
		recipients[*parentAuthor] = true
	}
	delete(recipients, u.ID)
	for rid := range recipients {
		a.notify(r, rid, "comment", map[string]any{
			"task_id": taskID, "task_title": title, "comment_id": id, "by": u.Username,
		})
	}
	// Watchers follow the discussion too; notifyWatchers skips the actor and the assignee, and
	// anyone already in `recipients` is filtered there as well.
	a.notifyWatchers(r, taskID, u.ID, "comment", map[string]any{
		"task_id": taskID, "task_title": title, "comment_id": id, "by": u.Username,
	})
	a.publishToListMembers(r, listID, events.Event{Type: "comment.created", Data: map[string]any{"task_id": taskID, "comment_id": id}})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// PATCH /api/comments/{id} {body} — author only (not admin: silently rewriting someone else's
// words, even for moderation, is a different action from deleting them — that stays admin-capable
// below). Sets edited_at so the UI can show an "(edited)" marker.
func (a *API) handleUpdateComment(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := readJSON(r, &in); err != nil || in.Body == "" {
		errJSON(w, http.StatusBadRequest, "comment cannot be empty")
		return
	}
	maxLen := a.intSetting(r.Context(), "limits.content.comment_max_len", 4000)
	if maxLen > 0 && len(in.Body) > maxLen {
		errJSON(w, http.StatusBadRequest, fmt.Sprintf("comment is longer than %d characters", maxLen))
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(),
		`UPDATE comments SET body=$2, edited_at=now() WHERE id=$1 AND author_id=$3 AND deleted_at IS NULL AND is_system=false`,
		id, in.Body, u.ID)
	if err != nil || tag.RowsAffected() == 0 {
		errJSON(w, http.StatusForbidden, "you can only edit your own comments")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/comments/{id} — author or admin.
func (a *API) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(),
		`UPDATE comments SET deleted_at=now() WHERE id=$1 AND ($2 OR author_id=$3) AND is_system=false`, id, u.IsAdmin(), u.ID)
	if err != nil || tag.RowsAffected() == 0 {
		errJSON(w, http.StatusForbidden, "you can only delete your own comments")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/reactions {target_type: task|comment, target_id, emoji} — toggle.
func (a *API) handleToggleReaction(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if !a.featureEnabled(r.Context(), "reactions") {
		errJSON(w, http.StatusForbidden, "reactions are disabled on this server")
		return
	}
	var in struct {
		TargetType string `json:"target_type"`
		TargetID   int64  `json:"target_id"`
		Emoji      string `json:"emoji"`
	}
	if err := readJSON(r, &in); err != nil || (in.TargetType != "task" && in.TargetType != "comment") {
		errJSON(w, http.StatusBadRequest, "target_type: task | comment")
		return
	}
	if !AllowedReactions[in.Emoji] {
		errJSON(w, http.StatusBadRequest, "invalid reaction")
		return
	}
	// resolve the target's list to check access — matches the permission check comments use.
	var listID int64
	var lookupErr error
	if in.TargetType == "task" {
		lookupErr = a.DB.Pool.QueryRow(r.Context(), `SELECT list_id FROM tasks WHERE id=$1`, in.TargetID).Scan(&listID)
	} else {
		lookupErr = a.DB.Pool.QueryRow(r.Context(),
			`SELECT t.list_id FROM comments c JOIN tasks t ON t.id=c.task_id WHERE c.id=$1`, in.TargetID).Scan(&listID)
	}
	if lookupErr != nil {
		errJSON(w, http.StatusNotFound, "target not found")
		return
	}
	if !permAtLeast(a.listPermission(r, u, listID), "viewer") {
		errJSON(w, http.StatusForbidden, "no access")
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(),
		`DELETE FROM reactions WHERE target_type=$1 AND target_id=$2 AND user_id=$3 AND emoji=$4`,
		in.TargetType, in.TargetID, u.ID, in.Emoji)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if tag.RowsAffected() > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"toggled": "off"})
		return
	}
	if _, err := a.DB.Pool.Exec(r.Context(),
		`INSERT INTO reactions(target_type, target_id, user_id, emoji) VALUES($1,$2,$3,$4)`,
		in.TargetType, in.TargetID, u.ID, in.Emoji); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	// notify the target's author
	var authorID int64
	var q string
	if in.TargetType == "task" {
		q = `SELECT creator_id FROM tasks WHERE id=$1`
	} else {
		q = `SELECT author_id FROM comments WHERE id=$1`
	}
	if a.DB.Pool.QueryRow(r.Context(), q, in.TargetID).Scan(&authorID) == nil && authorID != u.ID {
		a.notify(r, authorID, "reaction", map[string]any{
			"target_type": in.TargetType, "target_id": in.TargetID, "emoji": in.Emoji, "by": u.Username,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"toggled": "on"})
}

// GET /api/notifications?unread=1&limit=50
func (a *API) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	onlyUnread := r.URL.Query().Get("unread") == "1"
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT id, kind, payload, read_at, created_at FROM notifications
		WHERE user_id=$1 AND ($2 = false OR read_at IS NULL)
		ORDER BY created_at DESC LIMIT 100`, u.ID, onlyUnread)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	locale := ""
	_ = a.DB.Pool.QueryRow(r.Context(), `SELECT COALESCE(locale, '') FROM users WHERE id=$1`, u.ID).Scan(&locale)
	locale = a.normalizeLocale(locale)
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var kind string
		var payloadJSON json.RawMessage
		var readAt, createdAt any
		if rows.Scan(&id, &kind, &payloadJSON, &readAt, &createdAt) == nil {
			list = append(list, notificationItem(locale, id, kind, payloadJSON, readAt, createdAt))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": list})
}

// notificationItem turns a stored notification row into the DTO consumed by NotificationsPage.
// Keeping this transformation separate makes the JSONB decoding and localization testable without
// a database, and prevents the frontend from having to know PostgreSQL's JSON representation.
func notificationItem(locale string, id int64, kind string, payloadJSON json.RawMessage, readAt, createdAt any) map[string]any {
	payload := map[string]any{}
	_ = json.Unmarshal(payloadJSON, &payload)
	item := map[string]any{
		"id": id, "kind": kind, "payload": payload,
		"text": formatNotifText(locale, kind, payload),
		"read_at": readAt, "created_at": createdAt,
	}
	if taskID, ok := toInt(payload["task_id"]); ok {
		item["task_id"] = taskID
	}
	return item
}

// POST /api/notifications/read {ids?: []} — marks all as read when ids is omitted.
func (a *API) handleReadNotifications(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var in struct {
		IDs []int64 `json:"ids"`
	}
	_ = readJSON(r, &in)
	if len(in.IDs) == 0 {
		_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE notifications SET read_at=now() WHERE user_id=$1 AND read_at IS NULL`, u.ID)
	} else {
		_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE notifications SET read_at=now() WHERE user_id=$1 AND id=ANY($2)`, u.ID, in.IDs)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
