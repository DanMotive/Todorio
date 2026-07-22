package api

import (
	"net/http"
	"time"

	"github.com/DanMotive/Todorio/internal/events"
)

// POST /api/announcements — объявление на весь сервер (только root, space_id не указан)
// или на пространство (владелец пространства или админ).
func (a *API) handleCreateAnnouncement(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var in struct {
		SpaceID     *int64 `json:"space_id"`
		Level       string `json:"level"` // normal | important | emergency
		Body        string `json:"body"`
		RequiresAck bool   `json:"requires_ack"`
		ExpiresDays *int   `json:"expires_days"`
	}
	if err := readJSON(r, &in); err != nil || in.Body == "" {
		errJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	switch in.Level {
	case "":
		in.Level = "normal"
	case "normal", "important", "emergency":
	default:
		errJSON(w, http.StatusBadRequest, "level: normal | important | emergency")
		return
	}
	if in.SpaceID == nil {
		if u.Role != "root" {
			errJSON(w, http.StatusForbidden, "объявления на весь сервер делает только root")
			return
		}
	} else if a.spaceRole(r, u.ID, u.IsAdmin(), *in.SpaceID) != "owner" {
		errJSON(w, http.StatusForbidden, "нужны права владельца пространства")
		return
	}

	var expires *time.Time
	if in.ExpiresDays != nil && *in.ExpiresDays > 0 {
		t := time.Now().AddDate(0, 0, *in.ExpiresDays)
		expires = &t
	}
	var id int64
	if err := a.DB.Pool.QueryRow(r.Context(), `
		INSERT INTO announcements(space_id, author_id, level, body, requires_ack, expires_at)
		VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
		in.SpaceID, u.ID, in.Level, in.Body, in.RequiresAck, expires).Scan(&id); err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}

	// Получатели: весь сервер или участники пространства. Баннер приедет по SSE всем,
	// в колокольчик пишем только important/emergency, чтобы не спамить.
	query := `SELECT id FROM users WHERE status='active' AND archived_at IS NULL`
	args := []any{}
	if in.SpaceID != nil {
		query = `SELECT user_id FROM space_members WHERE space_id=$1`
		args = append(args, *in.SpaceID)
	}
	rows, err := a.DB.Pool.Query(r.Context(), query, args...)
	if err == nil {
		defer rows.Close()
		ids := []int64{}
		for rows.Next() {
			var uid int64
			if rows.Scan(&uid) == nil {
				ids = append(ids, uid)
			}
		}
		payload := map[string]any{"id": id, "level": in.Level, "body": in.Body}
		if in.Level != "normal" {
			for _, uid := range ids {
				if uid != u.ID {
					a.notify(r, uid, "announcement", payload)
				}
			}
		} else {
			a.Bus.Publish(ids, events.Event{Type: "announcement", Data: payload})
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// GET /api/announcements/active — актуальные для текущего пользователя (не скрытые и не истёкшие).
func (a *API) handleActiveAnnouncements(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT an.id, an.space_id, an.level, an.body, an.requires_ack, an.created_at
		FROM announcements an
		WHERE (an.expires_at IS NULL OR an.expires_at > now())
		  AND (an.space_id IS NULL OR an.space_id IN (SELECT space_id FROM space_members WHERE user_id=$1))
		  AND NOT EXISTS (SELECT 1 FROM announcement_acks k WHERE k.announcement_id = an.id AND k.user_id = $1)
		ORDER BY CASE an.level WHEN 'emergency' THEN 0 WHEN 'important' THEN 1 ELSE 2 END, an.id DESC
		LIMIT 20`, u.ID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var (
			id          int64
			spaceID     *int64
			level, body string
			requiresAck bool
			createdAt   any
		)
		if rows.Scan(&id, &spaceID, &level, &body, &requiresAck, &createdAt) == nil {
			list = append(list, map[string]any{
				"id": id, "space_id": spaceID, "level": level, "body": body,
				"requires_ack": requiresAck, "created_at": createdAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"announcements": list})
}

// POST /api/announcements/{id}/ack — «прочитал/скрыть».
func (a *API) handleAckAnnouncement(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный id")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `
		INSERT INTO announcement_acks(announcement_id, user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, u.ID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
