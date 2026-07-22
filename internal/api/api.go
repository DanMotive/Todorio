// Package api — HTTP-хэндлеры Todorio (без публичного API: только для собственного фронтенда,
// авторизация через cookie-сессии).
package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/DanMotive/Todorio/internal/auth"
	"github.com/DanMotive/Todorio/internal/config"
	"github.com/DanMotive/Todorio/internal/db"
	"github.com/DanMotive/Todorio/internal/events"
)

// Фиксированный набор реакций из ТЗ.
var AllowedReactions = map[string]bool{
	"\U0001F44D": true, "\u2705": true, "\U0001F389": true, "\U0001F525": true, "\U0001F440": true,
	"\u2753": true, "\u2757": true, "\u274C": true, "\U0001F62D": true, "\u2B50": true,
}

type API struct {
	DB      *db.DB
	Bus     *events.Bus
	Cfg     config.Config
	Version string
}

func (a *API) Routes(mux *http.ServeMux) {
	// --- аутентификация и профиль ---
	mux.HandleFunc("POST /api/register", a.handleRegister)
	mux.HandleFunc("POST /api/login", a.handleLogin)
	mux.HandleFunc("POST /api/logout", a.handleLogout)
	mux.HandleFunc("GET /api/me", a.handleMe)
	mux.HandleFunc("PATCH /api/me", a.handleUpdateMe)
	mux.HandleFunc("POST /api/me/password", a.handleChangePassword)

	// --- администрирование пользователей ---
	mux.HandleFunc("GET /api/admin/users", a.handleAdminUsers)
	mux.HandleFunc("POST /api/admin/users/{id}/approve", a.handleApproveUser)
	mux.HandleFunc("POST /api/admin/users/{id}/status", a.handleSetUserStatus)
	mux.HandleFunc("POST /api/admin/users/{id}/reset-password", a.handleResetPassword)

	// --- пространства и списки ---
	mux.HandleFunc("GET /api/spaces", a.handleListSpaces)
	mux.HandleFunc("POST /api/spaces", a.handleCreateSpace)
	mux.HandleFunc("PATCH /api/spaces/{id}", a.handleUpdateSpace)
	mux.HandleFunc("DELETE /api/spaces/{id}", a.handleArchiveSpace)
	mux.HandleFunc("POST /api/spaces/{id}/members", a.handleAddSpaceMember)
	mux.HandleFunc("GET /api/spaces/{id}/lists", a.handleListLists)
	mux.HandleFunc("POST /api/spaces/{id}/lists", a.handleCreateList)
	mux.HandleFunc("PATCH /api/lists/{id}", a.handleUpdateList)
	mux.HandleFunc("DELETE /api/lists/{id}", a.handleArchiveList)
	mux.HandleFunc("POST /api/lists/{id}/members", a.handleAddListMember)

	// --- задачи ---
	mux.HandleFunc("GET /api/lists/{id}/tasks", a.handleListTasks)
	mux.HandleFunc("POST /api/lists/{id}/tasks", a.handleCreateTask)
	mux.HandleFunc("GET /api/tasks/{id}", a.handleGetTask)
	mux.HandleFunc("PATCH /api/tasks/{id}", a.handleUpdateTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", a.handleArchiveTask)
	mux.HandleFunc("GET /api/my/tasks", a.handleMyTasks)

	// --- взаимодействия ---
	mux.HandleFunc("GET /api/tasks/{id}/comments", a.handleListComments)
	mux.HandleFunc("POST /api/tasks/{id}/comments", a.handleCreateComment)
	mux.HandleFunc("DELETE /api/comments/{id}", a.handleDeleteComment)
	mux.HandleFunc("POST /api/reactions", a.handleToggleReaction)

	// --- уведомления и реалтайм ---
	mux.HandleFunc("GET /api/notifications", a.handleListNotifications)
	mux.HandleFunc("POST /api/notifications/read", a.handleReadNotifications)
	mux.HandleFunc("GET /api/events", a.handleSSE)

	// --- Пульс пространства и статистика ---
	mux.HandleFunc("GET /api/spaces/{id}/pulse", a.handlePulse)
	mux.HandleFunc("GET /api/spaces/{id}/stats", a.handleStats)

	// --- TOTP (двухфакторка root/админов) ---
	mux.HandleFunc("POST /api/me/totp/setup", a.handleTOTPSetup)
	mux.HandleFunc("POST /api/me/totp/enable", a.handleTOTPEnable)
	mux.HandleFunc("POST /api/me/totp/disable", a.handleTOTPDisable)

	// --- вложения-картинки ---
	mux.HandleFunc("POST /api/tasks/{id}/attachments", a.handleUploadAttachment)
	mux.HandleFunc("GET /api/tasks/{id}/attachments", a.handleListAttachments)
	mux.HandleFunc("GET /api/attachments/{id}", a.handleGetAttachment)
	mux.HandleFunc("DELETE /api/attachments/{id}", a.handleDeleteAttachment)

	// --- объявления ---
	mux.HandleFunc("POST /api/announcements", a.handleCreateAnnouncement)
	mux.HandleFunc("GET /api/announcements/active", a.handleActiveAnnouncements)
	mux.HandleFunc("POST /api/announcements/{id}/ack", a.handleAckAnnouncement)

	// --- шаблоны списков ---
	mux.HandleFunc("POST /api/admin/templates", a.handleCreateTemplate)
	mux.HandleFunc("DELETE /api/admin/templates/{id}", a.handleDeleteTemplate)
	mux.HandleFunc("GET /api/templates", a.handleListTemplates)
	mux.HandleFunc("POST /api/templates/{id}/apply", a.handleApplyTemplate)

	// --- дайджест «пока вас не было» ---
	mux.HandleFunc("GET /api/digest", a.handleDigest)
	mux.HandleFunc("POST /api/digest/dismiss", a.handleDigestDismiss)
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// requireUser — только активный пользователь. pending видит только /api/me и смену пароля.
func (a *API) requireUser(w http.ResponseWriter, r *http.Request) *auth.User {
	u := auth.FromContext(r.Context())
	if u == nil {
		errJSON(w, http.StatusUnauthorized, "требуется вход")
		return nil
	}
	if u.Status != "active" {
		errJSON(w, http.StatusForbidden, "аккаунт ожидает одобрения или заблокирован")
		return nil
	}
	return u
}

func (a *API) requireAdmin(w http.ResponseWriter, r *http.Request) *auth.User {
	u := a.requireUser(w, r)
	if u == nil {
		return nil
	}
	if !u.IsAdmin() {
		errJSON(w, http.StatusForbidden, "нужны права администратора")
		return nil
	}
	return u
}

// listPermission возвращает право пользователя на список: owner | editor | viewer | "" (нет доступа).
// Root/admin получают owner.
func (a *API) listPermission(r *http.Request, u *auth.User, listID int64) string {
	if u.IsAdmin() {
		return "owner"
	}
	var perm string
	err := a.DB.Pool.QueryRow(r.Context(),
		`SELECT permission FROM list_members WHERE list_id=$1 AND user_id=$2`, listID, u.ID).Scan(&perm)
	if err != nil {
		return ""
	}
	return perm
}

func permAtLeast(perm, min string) bool {
	rank := map[string]int{"": 0, "viewer": 1, "editor": 2, "owner": 3}
	return rank[perm] >= rank[min]
}

// notify создаёт уведомление в БД и публикует в SSE.
func (a *API) notify(r *http.Request, userID int64, kind string, payload map[string]any) {
	b, _ := json.Marshal(payload)
	_, _ = a.DB.Pool.Exec(r.Context(),
		`INSERT INTO notifications(user_id, kind, payload) VALUES($1,$2,$3)`, userID, kind, string(b))
	a.Bus.Publish([]int64{userID}, events.Event{Type: "notification", Data: map[string]any{"kind": kind, "payload": payload}})
}
