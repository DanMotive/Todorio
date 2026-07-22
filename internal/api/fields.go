package api

// Custom fields: the space owner defines them in
// spaces.settings -> {"fields":[{"key","label","type":text|number|date|select|multiselect|checkbox|user|link|rating,"options","color"}]}.
// Values live per task in tasks.custom_fields (JSONB keyed by field key). Labels are just a
// "multiselect" field (conventionally keyed "labels") with colored options — no separate labels system.

import (
	"encoding/json"
	"net/http"
)

var allowedFieldTypes = map[string]bool{
	"text": true, "number": true, "date": true, "select": true, "multiselect": true,
	"checkbox": true, "user": true, "link": true, "rating": true,
}

// GET /api/spaces/{id}/fields
func (a *API) handleGetFields(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "no access to the space")
		return
	}
	var raw *string
	_ = a.DB.Pool.QueryRow(r.Context(),
		`SELECT settings #>> '{fields}' FROM spaces WHERE id=$1`, spaceID).Scan(&raw)
	var fields []map[string]any
	if raw != nil {
		_ = json.Unmarshal([]byte(*raw), &fields)
	}
	if fields == nil {
		fields = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"fields": fields})
}

// PUT /api/spaces/{id}/fields {fields: [...]} — space owner only; replaces the whole field schema.
func (a *API) handleSetFields(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) != "owner" {
		errJSON(w, http.StatusForbidden, "space owner permission required")
		return
	}
	var in struct {
		Fields []struct {
			Key     string   `json:"key"`
			Label   string   `json:"label"`
			Type    string   `json:"type"`
			Options []string `json:"options,omitempty"`
			Color   string   `json:"color,omitempty"`
		} `json:"fields"`
	}
	if err := readJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	for _, f := range in.Fields {
		if f.Key == "" || f.Label == "" || !allowedFieldTypes[f.Type] {
			errJSON(w, http.StatusBadRequest, "each field needs a key, label, and a valid type")
			return
		}
	}
	b, _ := json.Marshal(in.Fields)
	_, err = a.DB.Pool.Exec(r.Context(), `
		UPDATE spaces SET settings = jsonb_set(settings, '{fields}', $2::jsonb) WHERE id=$1`,
		spaceID, string(b))
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
