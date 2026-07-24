package api

// Onboarding quest progress bar (spec section 12: "обучающие задания ... с прогресс-баром
// освоения"). The quests themselves (internal/demo.Quests) and the "add to a new list on
// approval" step already existed; only the progress readout was missing.
//
// There is no dedicated flag marking "this is the quest list" — postApprove creates it with a
// fixed name ("Onboarding quests") owned by the newly approved user. That's enough to find it
// unambiguously: the demo space's own list of the same name is owned by root via space
// membership, not by an individual list_members "owner" row, so scoping to the caller's own
// owned list never confuses the two.

import "net/http"

// GET /api/onboarding/progress — done/total for the caller's own onboarding quest list, if any.
func (a *API) handleOnboardingProgress(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	var listID, total, done int
	err := a.DB.Pool.QueryRow(r.Context(), `
		SELECT l.id,
			(SELECT count(*) FROM tasks t WHERE t.list_id=l.id AND t.archived_at IS NULL),
			(SELECT count(*) FROM tasks t WHERE t.list_id=l.id AND t.archived_at IS NULL AND t.completed_at IS NOT NULL)
		FROM lists l
		JOIN list_members lm ON lm.list_id = l.id AND lm.user_id = $1 AND lm.permission = 'owner'
		WHERE l.name = 'Onboarding quests' AND l.archived_at IS NULL
		ORDER BY l.id LIMIT 1`, u.ID).Scan(&listID, &total, &done)
	if err != nil || total == 0 {
		// No quest list (quests were off at approval time, it was archived, or this account
		// predates the feature) — the bar simply doesn't apply, not an error.
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": true, "list_id": listID, "done": done, "total": total,
	})
}
