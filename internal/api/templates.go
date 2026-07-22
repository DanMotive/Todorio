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
	var id int64
	if err := a.DB.Pool.QueryRow(r.Context(), `
		INSERT INTO templates(name, body, auto_apply) VALUES($1,$2,$3) RETURNING id`,
		in.Name, string(in.Body), in.AutoApply).Scan(&id); err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// GET /api/templates — templates visible to all active users.
func (a *API) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	if a.requireUser(w, r) == nil {
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(),
		`SELECT id, name, body, auto_apply FROM templates ORDER BY id`)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var name string
		var body json.RawMessage
		var autoApply bool
		if rows.Scan(&id, &name, &body, &autoApply) == nil {
			list = append(list, map[string]any{"id": id, "name": name, "body": body, "auto_apply": autoApply})
		}
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
	var raw []byte
	if a.DB.Pool.QueryRow(r.Context(), `SELECT body FROM templates WHERE id=$1`, id).Scan(&raw) != nil {
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
	var listID int64
	if err := a.DB.Pool.QueryRow(ctx,
		`INSERT INTO lists(space_id, name) VALUES($1,$2) RETURNING id`, spaceID, body.ListName).Scan(&listID); err != nil {
		return 0, err
	}
	_, _ = a.DB.Pool.Exec(ctx,
		`INSERT INTO list_members(list_id,user_id,permission) VALUES($1,$2,'owner') ON CONFLICT DO NOTHING`, listID, userID)
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
		_, _ = a.DB.Pool.Exec(ctx, `
			INSERT INTO tasks(list_id, title, description, priority, assignee_id, due_at, creator_id)
			VALUES($1,$2,$3,$4,$5,$6,$7)`,
			listID, t.Title, t.Description, priority, userID, due, userID)
	}
	return listID, nil
}

// postApprove — what happens after a new user is approved:
// access to the demo space, personal space, auto_apply templates, and onboarding quests.
func (a *API) postApprove(ctx context.Context, userID int64, username string) {
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
		"🌱 "+username+"'s space", userID).Scan(&spaceID); err != nil {
		return
	}
	_, _ = a.DB.Pool.Exec(ctx,
		`INSERT INTO space_members(space_id,user_id,role) VALUES($1,$2,'owner')`, spaceID, userID)

	// 3. root's auto_apply templates
	rows, err := a.DB.Pool.Query(ctx, `SELECT body FROM templates WHERE auto_apply`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if rows.Scan(&raw) == nil {
				_, _ = a.applyTemplate(ctx, raw, spaceID, userID)
			}
		}
	}

	// 4. onboarding quests (disable via onboarding.quests = "off")
	if a.DB.Setting(ctx, "onboarding.quests", "on") == "off" {
		return
	}
	var listID int64
	if err := a.DB.Pool.QueryRow(ctx,
		`INSERT INTO lists(space_id, name) VALUES($1,$2) RETURNING id`,
		spaceID, "🎯 Onboarding quests").Scan(&listID); err != nil {
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
