package api

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/DanMotive/Todorio/internal/events"
)

var mentionRe = regexp.MustCompile(`@([a-zA-Z0-9_]{3,32})`)

// GET /api/tasks/{id}/comments
func (a *API) handleListComments(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	taskID, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный id")
		return
	}
	var listID int64
	if a.DB.Pool.QueryRow(r.Context(), `SELECT list_id FROM tasks WHERE id=$1`, taskID).Scan(&listID) != nil {
		errJSON(w, http.StatusNotFound, "задача не найдена")
		return
	}
	if !permAtLeast(a.listPermission(r, u, listID), "viewer") {
		errJSON(w, http.StatusForbidden, "нет доступа")
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT c.id, c.author_id, u.username, c.body, c.created_at,
			COALESCE((SELECT json_agg(json_build_object('emoji', rx.emoji, 'user_id', rx.user_id))
				FROM reactions rx WHERE rx.target_type='comment' AND rx.target_id=c.id), '[]'::json)
		FROM comments c JOIN users u ON u.id=c.author_id
		WHERE c.task_id=$1 AND c.deleted_at IS NULL
		ORDER BY c.created_at`, taskID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	defer rows.Close()
	comments := []map[string]any{}
	for rows.Next() {
		var id, authorID int64
		var username, body string
		var createdAt, reactions any
		if rows.Scan(&id, &authorID, &username, &body, &createdAt, &reactions) == nil {
			comments = append(comments, map[string]any{
				"id": id, "author_id": authorID, "author": username, "body": body,
				"created_at": createdAt, "reactions": reactions,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": comments})
}

// POST /api/tasks/{id}/comments {body} — @упоминания шлют уведомления; исполнитель и автор задачи тоже уведомляются.
func (a *API) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	taskID, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный id")
		return
	}
	var listID, creatorID int64
	var assigneeID *int64
	var title string
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT list_id, creator_id, assignee_id, title FROM tasks WHERE id=$1 AND archived_at IS NULL`,
		taskID).Scan(&listID, &creatorID, &assigneeID, &title) != nil {
		errJSON(w, http.StatusNotFound, "задача не найдена")
		return
	}
	if !permAtLeast(a.listPermission(r, u, listID), "viewer") {
		errJSON(w, http.StatusForbidden, "нет доступа")
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := readJSON(r, &in); err != nil || in.Body == "" {
		errJSON(w, http.StatusBadRequest, "пустой комментарий")
		return
	}
	maxLen := 4000 // TODO: читать из policy.limits.comment_max_len
	if len(in.Body) > maxLen {
		errJSON(w, http.StatusBadRequest, fmt.Sprintf("комментарий длиннее %d символов", maxLen))
		return
	}
	var id int64
	if err := a.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO comments(task_id, author_id, body) VALUES($1,$2,$3) RETURNING id`,
		taskID, u.ID, in.Body).Scan(&id); err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}

	// адресаты: упомянутые + исполнитель + автор задачи (без дублей, без самого автора комментария)
	recipients := map[int64]bool{creatorID: true}
	if assigneeID != nil {
		recipients[*assigneeID] = true
	}
	for _, m := range mentionRe.FindAllStringSubmatch(in.Body, -1) {
		var mid int64
		if a.DB.Pool.QueryRow(r.Context(),
			`SELECT id FROM users WHERE username=$1 AND status='active'`, m[1]).Scan(&mid) == nil {
			recipients[mid] = true
		}
	}
	delete(recipients, u.ID)
	for rid := range recipients {
		a.notify(r, rid, "comment", map[string]any{
			"task_id": taskID, "task_title": title, "comment_id": id, "by": u.Username,
		})
	}
	a.publishToListMembers(r, listID, events.Event{Type: "comment.created", Data: map[string]any{"task_id": taskID, "comment_id": id}})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// DELETE /api/comments/{id} — автор или админ.
func (a *API) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный id")
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(),
		`UPDATE comments SET deleted_at=now() WHERE id=$1 AND ($2 OR author_id=$3)`, id, u.IsAdmin(), u.ID)
	if err != nil || tag.RowsAffected() == 0 {
		errJSON(w, http.StatusForbidden, "можно удалять только свои комментарии")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/reactions {target_type: task|comment, target_id, emoji} — тоггл.
func (a *API) handleToggleReaction(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
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
		errJSON(w, http.StatusBadRequest, "недопустимая реакция")
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(),
		`DELETE FROM reactions WHERE target_type=$1 AND target_id=$2 AND user_id=$3 AND emoji=$4`,
		in.TargetType, in.TargetID, u.ID, in.Emoji)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	if tag.RowsAffected() > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"toggled": "off"})
		return
	}
	if _, err := a.DB.Pool.Exec(r.Context(),
		`INSERT INTO reactions(target_type, target_id, user_id, emoji) VALUES($1,$2,$3,$4)`,
		in.TargetType, in.TargetID, u.ID, in.Emoji); err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	// уведомляем автора цели
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
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var kind string
		var payload, readAt, createdAt any
		if rows.Scan(&id, &kind, &payload, &readAt, &createdAt) == nil {
			list = append(list, map[string]any{"id": id, "kind": kind, "payload": payload, "read_at": readAt, "created_at": createdAt})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": list})
}

// POST /api/notifications/read {ids?: []} — без ids отмечает все.
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
