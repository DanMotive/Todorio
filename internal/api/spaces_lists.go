package api

import (
	"net/http"
)

// GET /api/spaces — пространства, где пользователь участник (root/admin видят все).
func (a *API) handleListSpaces(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	query := `
		SELECT s.id, s.name, s.owner_id, COALESCE(m.role,'') AS my_role, s.created_at
		FROM spaces s
		LEFT JOIN space_members m ON m.space_id = s.id AND m.user_id = $1
		WHERE s.archived_at IS NULL AND ($2 OR m.user_id IS NOT NULL)
		ORDER BY s.created_at`
	rows, err := a.DB.Pool.Query(r.Context(), query, u.ID, u.IsAdmin())
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	defer rows.Close()
	spaces := []map[string]any{}
	for rows.Next() {
		var id, ownerID int64
		var name, myRole string
		var createdAt any
		if rows.Scan(&id, &name, &ownerID, &myRole, &createdAt) == nil {
			spaces = append(spaces, map[string]any{"id": id, "name": name, "owner_id": ownerID, "my_role": myRole, "created_at": createdAt})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"spaces": spaces})
}

// POST /api/spaces {name} — политика users.can_create_spaces (дефолт true).
func (a *API) handleCreateSpace(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if !u.IsAdmin() && a.DB.Setting(r.Context(), "policy.users.can_create_spaces", "true") != "true" {
		errJSON(w, http.StatusForbidden, "создание пространств отключено администратором")
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &in); err != nil || in.Name == "" {
		errJSON(w, http.StatusBadRequest, "нужно имя пространства")
		return
	}
	var id int64
	err := a.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO spaces(name, owner_id) VALUES($1,$2) RETURNING id`, in.Name, u.ID).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(),
		`INSERT INTO space_members(space_id, user_id, role) VALUES($1,$2,'owner')`, id, u.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (a *API) spaceRole(r *http.Request, u int64, isAdmin bool, spaceID int64) string {
	if isAdmin {
		return "owner"
	}
	var role string
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT role FROM space_members WHERE space_id=$1 AND user_id=$2`, spaceID, u).Scan(&role) != nil {
		return ""
	}
	return role
}

// PATCH /api/spaces/{id} {name?, settings?} — только owner пространства (настройки: workflow, Пульс, рейтинги...).
func (a *API) handleUpdateSpace(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), id) != "owner" {
		errJSON(w, http.StatusForbidden, "нужны права владельца пространства")
		return
	}
	var in struct {
		Name     *string         `json:"name"`
		Settings *map[string]any `json:"settings"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	_, err = a.DB.Pool.Exec(r.Context(), `
		UPDATE spaces SET name = COALESCE($2, name),
			settings = COALESCE($3, settings)
		WHERE id=$1`, id, in.Name, in.Settings)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/spaces/{id} — в архив (автоочистка через 30 дней — worker).
func (a *API) handleArchiveSpace(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), id) != "owner" {
		errJSON(w, http.StatusForbidden, "нужны права владельца пространства")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE spaces SET archived_at=now() WHERE id=$1`, id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/spaces/{id}/members {username, role}
func (a *API) handleAddSpaceMember(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), id) != "owner" {
		errJSON(w, http.StatusForbidden, "нужны права владельца пространства")
		return
	}
	var in struct {
		Username string `json:"username"`
		Role     string `json:"role"` // member | viewer | owner
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	if in.Role != "owner" && in.Role != "member" && in.Role != "viewer" {
		in.Role = "member"
	}
	var userID int64
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT id FROM users WHERE username=$1 AND status='active' AND archived_at IS NULL`, in.Username).Scan(&userID) != nil {
		errJSON(w, http.StatusNotFound, "активный пользователь не найден")
		return
	}
	_, err = a.DB.Pool.Exec(r.Context(), `
		INSERT INTO space_members(space_id, user_id, role) VALUES($1,$2,$3)
		ON CONFLICT (space_id, user_id) DO UPDATE SET role=$3`, id, userID, in.Role)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	a.notify(r, userID, "space_added", map[string]any{"space_id": id, "role": in.Role})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/spaces/{id}/lists — списки пространства, доступные пользователю.
func (a *API) handleListLists(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "нет доступа к пространству")
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT l.id, l.name, l.is_private, COALESCE(lm.permission,'') AS my_perm, l.position,
			(SELECT count(*) FROM tasks t WHERE t.list_id=l.id AND t.archived_at IS NULL) AS task_count,
			(SELECT count(*) FROM tasks t WHERE t.list_id=l.id AND t.archived_at IS NULL AND t.completed_at IS NOT NULL) AS done_count
		FROM lists l
		LEFT JOIN list_members lm ON lm.list_id=l.id AND lm.user_id=$2
		WHERE l.space_id=$1 AND l.archived_at IS NULL
			AND ($3 OR lm.user_id IS NOT NULL OR l.is_private=false)
		ORDER BY l.position, l.id`, spaceID, u.ID, u.IsAdmin())
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	defer rows.Close()
	lists := []map[string]any{}
	for rows.Next() {
		var id, position, taskCount, doneCount int64
		var name, myPerm string
		var isPrivate bool
		if rows.Scan(&id, &name, &isPrivate, &myPerm, &position, &taskCount, &doneCount) == nil {
			lists = append(lists, map[string]any{
				"id": id, "name": name, "is_private": isPrivate, "my_permission": myPerm,
				"position": position, "task_count": taskCount, "done_count": doneCount,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"lists": lists})
}

// POST /api/spaces/{id}/lists {name, is_private}
func (a *API) handleCreateList(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	role := a.spaceRole(r, u.ID, u.IsAdmin(), spaceID)
	if err != nil || (role != "owner" && role != "member") {
		errJSON(w, http.StatusForbidden, "нет прав на создание списка")
		return
	}
	var in struct {
		Name      string `json:"name"`
		IsPrivate bool   `json:"is_private"`
	}
	if err := readJSON(r, &in); err != nil || in.Name == "" {
		errJSON(w, http.StatusBadRequest, "нужно имя списка")
		return
	}
	var id int64
	err = a.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO lists(space_id, name, is_private) VALUES($1,$2,$3) RETURNING id`,
		spaceID, in.Name, in.IsPrivate).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(),
		`INSERT INTO list_members(list_id, user_id, permission) VALUES($1,$2,'owner')`, id, u.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// PATCH /api/lists/{id} {name?, settings?, position?}
func (a *API) handleUpdateList(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil || !permAtLeast(a.listPermission(r, u, id), "owner") {
		errJSON(w, http.StatusForbidden, "нужны права владельца списка")
		return
	}
	var in struct {
		Name     *string         `json:"name"`
		Settings *map[string]any `json:"settings"`
		Position *int            `json:"position"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	_, err = a.DB.Pool.Exec(r.Context(), `
		UPDATE lists SET name=COALESCE($2,name), settings=COALESCE($3,settings), position=COALESCE($4,position)
		WHERE id=$1`, id, in.Name, in.Settings, in.Position)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) handleArchiveList(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil || !permAtLeast(a.listPermission(r, u, id), "owner") {
		errJSON(w, http.StatusForbidden, "нужны права владельца списка")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `UPDATE lists SET archived_at=now() WHERE id=$1`, id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/lists/{id}/members {username, permission}
func (a *API) handleAddListMember(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	if err != nil || !permAtLeast(a.listPermission(r, u, id), "owner") {
		errJSON(w, http.StatusForbidden, "нужны права владельца списка")
		return
	}
	var in struct {
		Username   string `json:"username"`
		Permission string `json:"permission"` // owner | editor | viewer
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	if in.Permission != "owner" && in.Permission != "editor" && in.Permission != "viewer" {
		in.Permission = "viewer"
	}
	var userID int64
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT id FROM users WHERE username=$1 AND status='active' AND archived_at IS NULL`, in.Username).Scan(&userID) != nil {
		errJSON(w, http.StatusNotFound, "активный пользователь не найден")
		return
	}
	_, err = a.DB.Pool.Exec(r.Context(), `
		INSERT INTO list_members(list_id, user_id, permission) VALUES($1,$2,$3)
		ON CONFLICT (list_id, user_id) DO UPDATE SET permission=$3`, id, userID, in.Permission)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "ошибка БД")
		return
	}
	a.notify(r, userID, "list_shared", map[string]any{"list_id": id, "permission": in.Permission})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
