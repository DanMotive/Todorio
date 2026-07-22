package api

// Favorites: tasks or lists a user pins for quick access (starred in the sidebar).

import "net/http"

// GET /api/favorites
func (a *API) handleListFavorites(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	rows, err := a.DB.Pool.Query(r.Context(),
		`SELECT target_type, target_id FROM favorites WHERE user_id=$1 ORDER BY created_at DESC`, u.ID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	defer rows.Close()
	favs := []map[string]any{}
	for rows.Next() {
		var targetType string
		var targetID int64
		if rows.Scan(&targetType, &targetID) == nil {
			favs = append(favs, map[string]any{"target_type": targetType, "target_id": targetID})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"favorites": favs})
}

// POST /api/favorites {target_type: task|list, target_id} — toggle.
func (a *API) handleToggleFavorite(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var in struct {
		TargetType string `json:"target_type"`
		TargetID   int64  `json:"target_id"`
	}
	if err := readJSON(r, &in); err != nil || (in.TargetType != "task" && in.TargetType != "list") {
		errJSON(w, http.StatusBadRequest, "target_type: task | list")
		return
	}
	tag, err := a.DB.Pool.Exec(r.Context(),
		`DELETE FROM favorites WHERE user_id=$1 AND target_type=$2 AND target_id=$3`, u.ID, in.TargetType, in.TargetID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	if tag.RowsAffected() > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"toggled": "off"})
		return
	}
	if _, err := a.DB.Pool.Exec(r.Context(),
		`INSERT INTO favorites(user_id, target_type, target_id) VALUES($1,$2,$3)`, u.ID, in.TargetType, in.TargetID); err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"toggled": "on"})
}
