package api

import "net/http"

// Member management — the read/update/revoke half of the permission model.
//
// POST /api/spaces/{id}/members and POST /api/lists/{id}/members already lived in
// spaces_lists.go, but nothing could *read* who has access, change someone's role, or take it
// away again. That made the whole model (space owner/member/viewer, list owner/editor/viewer)
// unusable from a browser: the only way to see the roster was to query the database directly,
// and a mis-typed grant could never be undone. These handlers close that gap; the grant handlers
// stay where they are so existing call sites and tests keep working.
//
// Every write is owner-only, mirroring the existing grant handlers, and every write goes through
// lastOwner* below: a space or list whose last owner removes or demotes themselves would be left
// with no one able to administer it again, which is not recoverable through the API.

// memberRow scans one roster row. The account's *global* role is included because it changes how
// the space-scoped role behaves — spaceRole and listPermission both cap a globally read-only
// account at viewer, so a UI that showed only the space role would claim someone is an editor
// while every write of theirs is rejected.
func scanMembers(rows interface {
	Next() bool
	Scan(...any) error
	Close()
}) []map[string]any {
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var username, globalRole, status, scoped string
		var displayName *string
		if rows.Scan(&id, &username, &displayName, &globalRole, &status, &scoped) == nil {
			out = append(out, map[string]any{
				"user_id": id, "username": username, "display_name": displayName,
				"global_role": globalRole, "status": status, "role": scoped,
				// A globally read-only account cannot write regardless of the scoped role;
				// surfacing it here keeps the UI honest instead of making it re-derive the rule.
				"read_only": globalRole == "viewer",
			})
		}
	}
	return out
}

// GET /api/spaces/{id}/members — readable by any member; can_manage tells the client whether to
// render the editing controls at all.
func (a *API) handleListSpaceMembers(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	role := a.spaceRole(r, u.ID, u.IsAdmin(), id)
	if err != nil || role == "" {
		errJSON(w, http.StatusForbidden, "no access to the space")
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT u.id, u.username, u.display_name, u.role, u.status, sm.role
		FROM space_members sm
		JOIN users u ON u.id = sm.user_id
		WHERE sm.space_id=$1 AND u.archived_at IS NULL
		ORDER BY CASE sm.role WHEN 'owner' THEN 0 WHEN 'member' THEN 1 ELSE 2 END, u.username`, id)
	if err != nil {
		dbFail(r, "list space members", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"members": scanMembers(rows), "my_role": role, "can_manage": role == "owner",
	})
}

// PATCH /api/spaces/{id}/members/{user_id} {role} — owner | member | viewer.
func (a *API) handleUpdateSpaceMember(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) != "owner" {
		errJSON(w, http.StatusForbidden, "space owner permission required")
		return
	}
	userID, err := pathIDNamed(r, "user_id")
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Role != "owner" && in.Role != "member" && in.Role != "viewer" {
		errJSON(w, http.StatusBadRequest, "allowed values: owner | member | viewer")
		return
	}
	if in.Role != "owner" && a.isLastSpaceOwner(r, spaceID, userID) {
		errJSON(w, http.StatusConflict, "the last owner of a space cannot be demoted — promote someone else first")
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(),
		`UPDATE space_members SET role=$3 WHERE space_id=$1 AND user_id=$2`, spaceID, userID, in.Role)
	if err != nil {
		dbFail(r, "update space member", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if tag.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "this user is not a member of the space")
		return
	}
	// Reuses the "space_added" notification kind rather than inventing a second one: from the
	// recipient's side a role change and being added are the same event ("your access to this
	// space is now X"), and the existing kind is already respected by notify_prefs.
	a.notify(r, userID, "space_added", map[string]any{"space_id": spaceID, "role": in.Role})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/spaces/{id}/members/{user_id}
func (a *API) handleRemoveSpaceMember(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) != "owner" {
		errJSON(w, http.StatusForbidden, "space owner permission required")
		return
	}
	userID, err := pathIDNamed(r, "user_id")
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if a.isLastSpaceOwner(r, spaceID, userID) {
		errJSON(w, http.StatusConflict, "the last owner of a space cannot be removed — promote someone else first")
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(),
		`DELETE FROM space_members WHERE space_id=$1 AND user_id=$2`, spaceID, userID)
	if err != nil {
		dbFail(r, "remove space member", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if tag.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "this user is not a member of the space")
		return
	}
	// Losing the space also means losing the lists inside it. Leaving list_members rows behind
	// would keep handing the removed user access to individual lists (listPermission never looks
	// at space membership), so revoking a space grant has to cascade or it isn't a revocation.
	_, _ = a.DB.Pool.Exec(r.Context(), `
		DELETE FROM list_members WHERE user_id=$2
			AND list_id IN (SELECT id FROM lists WHERE space_id=$1)`, spaceID, userID)
	// Their open tasks in the space are unassigned, the same way blocking a user does — an
	// assignee who can no longer open the task would otherwise silently stall it.
	_, _ = a.DB.Pool.Exec(r.Context(), `
		UPDATE tasks SET assignee_id=NULL
		WHERE assignee_id=$2 AND completed_at IS NULL
			AND list_id IN (SELECT id FROM lists WHERE space_id=$1)`, spaceID, userID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/lists/{id}/members
func (a *API) handleListListMembers(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	id, err := pathID(r)
	perm := a.listPermission(r, u, id)
	if err != nil || perm == "" {
		errJSON(w, http.StatusForbidden, "no access to the list")
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT u.id, u.username, u.display_name, u.role, u.status, lm.permission
		FROM list_members lm
		JOIN users u ON u.id = lm.user_id
		WHERE lm.list_id=$1 AND u.archived_at IS NULL
		ORDER BY CASE lm.permission WHEN 'owner' THEN 0 WHEN 'editor' THEN 1 ELSE 2 END, u.username`, id)
	if err != nil {
		dbFail(r, "list list members", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"members": scanMembers(rows), "my_permission": perm, "can_manage": permAtLeast(perm, "owner"),
	})
}

// PATCH /api/lists/{id}/members/{user_id} {permission} — owner | editor | viewer.
func (a *API) handleUpdateListMember(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	listID, err := pathID(r)
	if err != nil || !permAtLeast(a.listPermission(r, u, listID), "owner") {
		errJSON(w, http.StatusForbidden, "list owner permission required")
		return
	}
	userID, err := pathIDNamed(r, "user_id")
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var in struct {
		Permission string `json:"permission"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Permission != "owner" && in.Permission != "editor" && in.Permission != "viewer" {
		errJSON(w, http.StatusBadRequest, "allowed values: owner | editor | viewer")
		return
	}
	if in.Permission != "owner" && a.isLastListOwner(r, listID, userID) {
		errJSON(w, http.StatusConflict, "the last owner of a list cannot be demoted — promote someone else first")
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(),
		`UPDATE list_members SET permission=$3 WHERE list_id=$1 AND user_id=$2`, listID, userID, in.Permission)
	if err != nil {
		dbFail(r, "update list member", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if tag.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "this user is not a member of the list")
		return
	}
	a.notify(r, userID, "list_shared", map[string]any{"list_id": listID, "permission": in.Permission})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/lists/{id}/members/{user_id}
func (a *API) handleRemoveListMember(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	listID, err := pathID(r)
	if err != nil || !permAtLeast(a.listPermission(r, u, listID), "owner") {
		errJSON(w, http.StatusForbidden, "list owner permission required")
		return
	}
	userID, err := pathIDNamed(r, "user_id")
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if a.isLastListOwner(r, listID, userID) {
		errJSON(w, http.StatusConflict, "the last owner of a list cannot be removed — promote someone else first")
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(),
		`DELETE FROM list_members WHERE list_id=$1 AND user_id=$2`, listID, userID)
	if err != nil {
		dbFail(r, "remove list member", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	if tag.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "this user is not a member of the list")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `
		UPDATE tasks SET assignee_id=NULL
		WHERE list_id=$1 AND assignee_id=$2 AND completed_at IS NULL`, listID, userID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/lists/{id}/assignable — who this list's tasks may be assigned to.
//
// Until now the client had no way to learn this, so the task UI could only ever assign a task to
// the current user. The set is "members of the list" plus "members of its space" (a space member
// can already open every non-private list in it), minus globally read-only accounts, which are
// capped at viewer everywhere and so should never be handed work.
func (a *API) handleAssignableUsers(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	listID, err := pathID(r)
	if err != nil || a.listPermission(r, u, listID) == "" {
		errJSON(w, http.StatusForbidden, "no access to the list")
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT DISTINCT u.id, u.username, u.display_name
		FROM users u
		LEFT JOIN list_members lm ON lm.user_id = u.id AND lm.list_id = $1
		LEFT JOIN space_members sm ON sm.user_id = u.id
			AND sm.space_id = (SELECT space_id FROM lists WHERE id = $1)
		WHERE u.archived_at IS NULL AND u.status = 'active' AND u.role <> 'viewer'
			AND (lm.user_id IS NOT NULL OR sm.user_id IS NOT NULL)
		ORDER BY u.username`, listID)
	if err != nil {
		dbFail(r, "list assignable users", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	users := []map[string]any{}
	for rows.Next() {
		var id int64
		var username string
		var displayName *string
		if rows.Scan(&id, &username, &displayName) == nil {
			users = append(users, map[string]any{"id": id, "username": username, "display_name": displayName})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

// isLastSpaceOwner reports whether userID is currently the only owner of the space. Checked
// before any demotion or removal: a space with no owner left can't be renamed, shared, archived
// or restored by anyone but a server admin, and nothing in the API can put an owner back.
func (a *API) isLastSpaceOwner(r *http.Request, spaceID, userID int64) bool {
	var owners int
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT count(*) FROM space_members WHERE space_id=$1 AND role='owner'`, spaceID).Scan(&owners) != nil {
		return true // fail closed: if the count is unavailable, refuse rather than risk an ownerless space
	}
	if owners > 1 {
		return false
	}
	var isOwner bool
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT true FROM space_members WHERE space_id=$1 AND user_id=$2 AND role='owner'`, spaceID, userID).Scan(&isOwner) != nil {
		return false // the target isn't an owner at all, so nothing is being taken away
	}
	return isOwner
}

func (a *API) isLastListOwner(r *http.Request, listID, userID int64) bool {
	var owners int
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT count(*) FROM list_members WHERE list_id=$1 AND permission='owner'`, listID).Scan(&owners) != nil {
		return true
	}
	if owners > 1 {
		return false
	}
	var isOwner bool
	if a.DB.Pool.QueryRow(r.Context(),
		`SELECT true FROM list_members WHERE list_id=$1 AND user_id=$2 AND permission='owner'`, listID, userID).Scan(&isOwner) != nil {
		return false
	}
	return isOwner
}
