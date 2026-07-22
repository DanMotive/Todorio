package api

// Кастомные статусы воркфлоу: владелец пространства хранит их в
// spaces.settings -> {"workflow":{"statuses":["design","qa",...]}} (PATCH /api/spaces/{id}).
// Дефолтные статусы есть всегда; 'done' закрывает задачу (completed_at).

import (
	"encoding/json"
	"net/http"
)

var defaultStatuses = []string{"open", "in_progress", "review", "done"}

func mergeStatuses(rawJSON *string) []string {
	statuses := append([]string{}, defaultStatuses...)
	if rawJSON == nil {
		return statuses
	}
	var custom []string
	if json.Unmarshal([]byte(*rawJSON), &custom) != nil {
		return statuses
	}
	for _, c := range custom {
		if c == "" {
			continue
		}
		exists := false
		for _, s := range statuses {
			if s == c {
				exists = true
				break
			}
		}
		if !exists {
			statuses = append(statuses, c)
		}
	}
	return statuses
}

// listStatuses — допустимые статусы для задач списка (через его пространство).
func (a *API) listStatuses(r *http.Request, listID int64) []string {
	var raw *string
	_ = a.DB.Pool.QueryRow(r.Context(), `
		SELECT s.settings #>> '{workflow,statuses}'
		FROM spaces s JOIN lists l ON l.space_id = s.id
		WHERE l.id = $1`, listID).Scan(&raw)
	return mergeStatuses(raw)
}

// GET /api/spaces/{id}/workflow — набор статусов пространства (для селекторов в UI).
func (a *API) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "нет доступа к пространству")
		return
	}
	var raw *string
	_ = a.DB.Pool.QueryRow(r.Context(),
		`SELECT settings #>> '{workflow,statuses}' FROM spaces WHERE id=$1`, spaceID).Scan(&raw)
	writeJSON(w, http.StatusOK, map[string]any{
		"statuses": mergeStatuses(raw),
		"defaults": defaultStatuses,
	})
}
