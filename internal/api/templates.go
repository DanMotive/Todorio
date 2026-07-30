package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/DanMotive/Todorio/internal/demo"
)

// Template structure (templates.body): a ready-made list with tasks.
type templateBody struct {
	ListName string `json:"list_name"`
	Tasks    []struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Priority    string `json:"priority"`
		DueInDays   *int   `json:"due_in_days"`
	} `json:"tasks"`
}

// POST /api/admin/templates — only root can create templates (per spec).
func (a *API) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if u.Role != "root" {
		errJSON(w, http.StatusForbidden, "only root can create templates")
		return
	}
	var in struct {
		Name      string          `json:"name"`
		Body      json.RawMessage `json:"body"`
		AutoApply bool            `json:"auto_apply"`
		// Audience controls who sees the template (spec section 16: "публикация для всех
		// активных пользователей, определённых ролей или админов").
		Audience *templateAudience `json:"audience"`
	}
	if err := readJSON(r, &in); err != nil || in.Name == "" {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	var body templateBody
	if err := json.Unmarshal(in.Body, &body); err != nil || body.ListName == "" {
		errJSON(w, http.StatusBadRequest, "body: expected {list_name, tasks[]}")
		return
	}
	aud := templateAudience{Mode: "all"}
	if in.Audience != nil {
		aud = *in.Audience
	}
	if !validAudienceMode[aud.Mode] {
		errJSON(w, http.StatusBadRequest, "audience.mode must be all, roles, or admins")
		return
	}
	audJSON, _ := json.Marshal(aud)
	var id int64
	if err := a.DB.Pool.QueryRow(r.Context(), `
		INSERT INTO templates(name, body, auto_apply, audience) VALUES($1,$2,$3,$4) RETURNING id`,
		in.Name, string(in.Body), in.AutoApply, string(audJSON)).Scan(&id); err != nil {
		dbFail(r, "create template", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// templateAudience — who a published template is visible to.
//
//	{"mode":"all"}                        — every active user (the default)
//	{"mode":"admins"}                     — root and admins only
//	{"mode":"roles","roles":["user"]}     — the listed roles only
type templateAudience struct {
	Mode  string   `json:"mode"`
	Roles []string `json:"roles,omitempty"`
}

var validAudienceMode = map[string]bool{"all": true, "roles": true, "admins": true}

// visibleTo reports whether a user with the given role may see this template. An unset or
// unrecognised mode falls back to "visible to all" — matching the pre-audience behaviour, so
// templates created before this feature don't silently disappear.
func (t templateAudience) visibleTo(role string) bool {
	switch t.Mode {
	case "admins":
		return role == "root" || role == "admin"
	case "roles":
		for _, r := range t.Roles {
			if r == role {
				return true
			}
		}
		// Root always retains visibility — it manages templates and must be able to see
		// what it published, whatever the audience says.
		return role == "root"
	default:
		return true
	}
}

// GET /api/templates — templates the caller is allowed to see, per each template's audience.
func (a *API) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(),
		`SELECT id, name, body, auto_apply, audience FROM templates ORDER BY id`)
	if err != nil {
		dbFail(r, "list templates", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var name string
		var body, audRaw json.RawMessage
		var autoApply bool
		if rows.Scan(&id, &name, &body, &autoApply, &audRaw) != nil {
			continue
		}
		aud := templateAudience{Mode: "all"}
		if len(audRaw) > 0 {
			_ = json.Unmarshal(audRaw, &aud)
		}
		if !aud.visibleTo(u.Role) {
			continue
		}
		list = append(list, map[string]any{
			"id": id, "name": name, "body": body, "auto_apply": autoApply, "audience": aud,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": list})
}

// DELETE /api/admin/templates/{id}
func (a *API) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	if u.Role != "root" {
		errJSON(w, http.StatusForbidden, "only root can delete templates")
		return
	}
	id, err := pathID(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid id")
		return
	}
	_, _ = a.DB.Pool.Exec(r.Context(), `DELETE FROM templates WHERE id=$1`, id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/templates/{id}/apply {space_id} — instantiate the template in a space.
func (a *API) handleApplyTemplate(w http.ResponseWriter, r *http.Request) {
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
		SpaceID int64 `json:"space_id"`
	}
	if err := readJSON(r, &in); err != nil || in.SpaceID == 0 {
		errJSON(w, http.StatusBadRequest, "space_id is required")
		return
	}
	role := a.spaceRole(r, u.ID, u.IsAdmin(), in.SpaceID)
	if role != "owner" && role != "member" {
		errJSON(w, http.StatusForbidden, "no permission in the space")
		return
	}
	var raw, audRaw []byte
	if a.DB.Pool.QueryRow(r.Context(), `SELECT body, audience FROM templates WHERE id=$1`, id).Scan(&raw, &audRaw) != nil {
		errJSON(w, http.StatusNotFound, "template not found")
		return
	}
	// The audience is enforced here too, not just when listing: hiding a template from the
	// list is presentation, but applying it by guessed id has to be refused as well.
	aud := templateAudience{Mode: "all"}
	if len(audRaw) > 0 {
		_ = json.Unmarshal(audRaw, &aud)
	}
	if !aud.visibleTo(u.Role) {
		errJSON(w, http.StatusNotFound, "template not found")
		return
	}
	listID, err := a.applyTemplate(r.Context(), raw, in.SpaceID, u.ID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "failed to apply the template")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"list_id": listID})
}

// applyTemplate expands the template body into a new list in the space.
func (a *API) applyTemplate(ctx context.Context, raw []byte, spaceID, userID int64) (int64, error) {
	var body templateBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return 0, err
	}
	tx, err := a.DB.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var listID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO lists(space_id, name) VALUES($1,$2) RETURNING id`, spaceID, body.ListName).Scan(&listID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO list_members(list_id,user_id,permission) VALUES($1,$2,'owner') ON CONFLICT DO NOTHING`, listID, userID); err != nil {
		return 0, err
	}
	for _, t := range body.Tasks {
		priority := t.Priority
		if priority == "" {
			priority = "normal"
		}
		var due *time.Time
		if t.DueInDays != nil {
			d := time.Now().AddDate(0, 0, *t.DueInDays)
			due = &d
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO tasks(list_id, title, description, priority, assignee_id, due_at, creator_id)
			VALUES($1,$2,$3,$4,$5,$6,$7)`,
			listID, t.Title, t.Description, priority, userID, due, userID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return listID, nil
}

// postApprove — what happens after a new user is approved:
// access to the demo space, personal space, auto_apply templates, and onboarding quests.
// role is the role the user was just approved with — auto-apply must respect each template's
// audience, otherwise a template published to admins only would still be materialised into a
// plain user's personal space.
func (a *API) postApprove(ctx context.Context, userID int64, username, role string) {
	// 1. add to the demo space (if created during setup)
	if sid := a.DB.Setting(ctx, "onboarding.demo_space_id", ""); sid != "" {
		if demoID, err := strconv.ParseInt(sid, 10, 64); err == nil {
			_, _ = a.DB.Pool.Exec(ctx, `
				INSERT INTO space_members(space_id,user_id,role) VALUES($1,$2,'member') ON CONFLICT DO NOTHING`,
				demoID, userID)
		}
	}

	// 2. personal space
	var spaceID int64
	if err := a.DB.Pool.QueryRow(ctx,
		`INSERT INTO spaces(name, owner_id) VALUES($1,$2) RETURNING id`,
		username+"'s space", userID).Scan(&spaceID); err != nil {
		return
	}
	_, _ = a.DB.Pool.Exec(ctx,
		`INSERT INTO space_members(space_id,user_id,role) VALUES($1,$2,'owner')`, spaceID, userID)

	// 3. root's auto_apply templates (only those this user's role is allowed to see)
	rows, err := a.DB.Pool.Query(ctx, `SELECT body, audience FROM templates WHERE auto_apply`)
	if err == nil {
		type pending struct{ raw []byte }
		todo := []pending{}
		for rows.Next() {
			var raw, audRaw []byte
			if rows.Scan(&raw, &audRaw) != nil {
				continue
			}
			aud := templateAudience{Mode: "all"}
			if len(audRaw) > 0 {
				_ = json.Unmarshal(audRaw, &aud)
			}
			if aud.visibleTo(role) {
				todo = append(todo, pending{raw})
			}
		}
		rows.Close()
		// applyTemplate runs its own queries, so the rows cursor is drained and closed first
		// rather than issuing new queries while it's still open.
		for _, t := range todo {
			_, _ = a.applyTemplate(ctx, t.raw, spaceID, userID)
		}
	}

	// 4. onboarding quests (disable via onboarding.quests = "off")
	if a.DB.Setting(ctx, "onboarding.quests", "on") == "off" {
		return
	}
	var listID int64
	if err := a.DB.Pool.QueryRow(ctx,
		`INSERT INTO lists(space_id, name) VALUES($1,$2) RETURNING id`,
		spaceID, "Onboarding quests").Scan(&listID); err != nil {
		return
	}
	_, _ = a.DB.Pool.Exec(ctx,
		`INSERT INTO list_members(list_id,user_id,permission) VALUES($1,$2,'owner')`, listID, userID)
	for _, q := range demo.Quests() {
		_, _ = a.DB.Pool.Exec(ctx, `
			INSERT INTO tasks(list_id, title, description, priority, assignee_id, creator_id)
			VALUES($1,$2,$3,'normal',$4,$4)`, listID, q.Title, q.Description, userID)
	}
}
