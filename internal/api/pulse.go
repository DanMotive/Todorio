package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Space Pulse (spec section 17) — a live health summary computed from facts only, no AI and
// no external API.
//
// Everything configurable lives in the space's own settings JSONB under the "pulse" key, so a
// space owner tunes their own thresholds while root keeps a global on/off switch
// (pulse.enabled in system_settings). Defaults reproduce the previously hardcoded behaviour,
// so a space that has never been configured behaves exactly as before.
type pulseSettings struct {
	// StaleDays — days without an update before an open task counts as stalled.
	StaleDays int `json:"stale_days"`
	// GreenAt/YellowAt — score thresholds for the mood indicator. score >= GreenAt is green,
	// >= YellowAt is yellow, below that red.
	GreenAt  int `json:"green_at"`
	YellowAt int `json:"yellow_at"`
	// Signals toggles which signals are collected and shown. A disabled signal is omitted from
	// the response and excluded from the score, so turning off a signal a team doesn't care
	// about (e.g. "no deadline" for a support backlog) stops it dragging the score down.
	Signals map[string]bool `json:"signals"`
	// Standup controls the daily mini-standup block (spec section 17: "дополняет: ежедневный
	// мини-стендап").
	Standup bool `json:"standup"`
}

// pulseSignalNames is the authoritative signal list. Kept as a slice (not just the map) so the
// order in the API response is stable for the UI.
var pulseSignalNames = []string{"overdue", "unassigned", "no_deadline", "blocked", "stale"}

func defaultPulseSettings() pulseSettings {
	sig := make(map[string]bool, len(pulseSignalNames))
	for _, n := range pulseSignalNames {
		sig[n] = true
	}
	return pulseSettings{StaleDays: 3, GreenAt: 70, YellowAt: 40, Signals: sig, Standup: true}
}

// loadPulseSettings reads spaces.settings->'pulse'. Any field that is absent or out of range
// falls back to its default: a half-filled settings object from an older client must not be
// able to produce a nonsensical Pulse (e.g. stale_days = 0 would mark every task stalled).
func (a *API) loadPulseSettings(ctx context.Context, spaceID int64) pulseSettings {
	ps := defaultPulseSettings()
	var raw []byte
	if err := a.DB.Pool.QueryRow(ctx,
		`SELECT settings->'pulse' FROM spaces WHERE id=$1`, spaceID).Scan(&raw); err != nil || len(raw) == 0 {
		return ps
	}
	var in struct {
		StaleDays *int            `json:"stale_days"`
		GreenAt   *int            `json:"green_at"`
		YellowAt  *int            `json:"yellow_at"`
		Signals   map[string]bool `json:"signals"`
		Standup   *bool           `json:"standup"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return ps
	}
	if in.StaleDays != nil && *in.StaleDays >= 1 && *in.StaleDays <= 365 {
		ps.StaleDays = *in.StaleDays
	}
	// Thresholds must stay ordered (yellow < green) and inside 0..100, otherwise the mood
	// mapping below would be unreachable in one direction.
	if in.GreenAt != nil && *in.GreenAt >= 0 && *in.GreenAt <= 100 {
		ps.GreenAt = *in.GreenAt
	}
	if in.YellowAt != nil && *in.YellowAt >= 0 && *in.YellowAt <= 100 {
		ps.YellowAt = *in.YellowAt
	}
	if ps.YellowAt > ps.GreenAt {
		ps.YellowAt = ps.GreenAt
	}
	for _, n := range pulseSignalNames {
		if v, ok := in.Signals[n]; ok {
			ps.Signals[n] = v
		}
	}
	if in.Standup != nil {
		ps.Standup = *in.Standup
	}
	return ps
}

// nextAction is the "следующее лучшее действие" from the spec mockup: one concrete, actionable
// suggestion rather than a wall of numbers.
type nextAction struct {
	Kind   string `json:"kind"` // assign | unblock | schedule | overdue | nudge
	TaskID int64  `json:"task_id"`
	Title  string `json:"title"`
}

// GET /api/spaces/{id}/pulse
func (a *API) handlePulse(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "no access to the space")
		return
	}
	// Root's global kill switch (registered in settings.go as "pulse.enabled") wins over any
	// per-space configuration.
	if a.DB.Setting(r.Context(), "pulse.enabled", "true") == "false" {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	ctx := r.Context()
	ps := a.loadPulseSettings(ctx, spaceID)

	var total, open, done, overdue, unassigned, noDeadline, blocked, stale int
	// stale_days is interpolated as a bound parameter into make_interval rather than into the
	// SQL text — no string concatenation into the query.
	err = a.DB.Pool.QueryRow(ctx, `
		SELECT
			count(*)::int,
			count(*) FILTER (WHERE t.completed_at IS NULL)::int,
			count(*) FILTER (WHERE t.completed_at IS NOT NULL)::int,
			count(*) FILTER (WHERE t.completed_at IS NULL AND t.due_at < now())::int,
			count(*) FILTER (WHERE t.completed_at IS NULL AND t.assignee_id IS NULL)::int,
			count(*) FILTER (WHERE t.completed_at IS NULL AND t.due_at IS NULL)::int,
			count(*) FILTER (WHERE t.completed_at IS NULL AND COALESCE(array_length(t.blocked_by,1),0) > 0)::int,
			count(*) FILTER (WHERE t.completed_at IS NULL AND t.updated_at < now() - make_interval(days => $2))::int
		FROM tasks t JOIN lists l ON l.id = t.list_id
		WHERE l.space_id=$1 AND t.archived_at IS NULL AND l.archived_at IS NULL`,
		spaceID, ps.StaleDays).Scan(&total, &open, &done, &overdue, &unassigned, &noDeadline, &blocked, &stale)
	if err != nil {
		dbFail(r, "pulse counts", err)
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}

	// Health score 0..100: penalties for overdue/stalled/blocked/unassigned work. A signal the
	// owner switched off contributes nothing, so the score reflects what the team tracks.
	score := 100
	if open > 0 {
		if ps.Signals["overdue"] {
			score -= min(50, overdue*100/open/2)
		}
		if ps.Signals["stale"] {
			score -= min(25, stale*100/open/4)
		}
		if ps.Signals["blocked"] {
			score -= min(15, blocked*100/open/4)
		}
		if ps.Signals["unassigned"] {
			score -= min(10, unassigned*100/open/10)
		}
	}
	if score < 0 {
		score = 0
	}
	// mood is a plain color keyword (not an emoji) — the frontend draws its own pulsing/static
	// indicator dot from it, so rendering is consistent across platforms instead of depending
	// on the OS/browser's emoji font.
	mood := "green"
	if score < ps.GreenAt {
		mood = "yellow"
	}
	if score < ps.YellowAt {
		mood = "red"
	}

	// Only report signals the owner left enabled.
	counts := map[string]int{
		"overdue": overdue, "unassigned": unassigned, "no_deadline": noDeadline,
		"blocked": blocked, "stale": stale,
	}
	signals := make(map[string]int, len(counts))
	for _, n := range pulseSignalNames {
		if ps.Signals[n] {
			signals[n] = counts[n]
		}
	}

	resp := map[string]any{
		"enabled": true,
		"score":   score, "mood": mood,
		"total": total, "open": open, "done": done,
		"signals":  signals,
		"settings": map[string]any{"stale_days": ps.StaleDays, "green_at": ps.GreenAt, "yellow_at": ps.YellowAt, "standup": ps.Standup},
	}
	if act := a.pulseNextAction(ctx, spaceID, ps); act != nil {
		resp["next_action"] = act
	}
	resp["in_progress"] = a.pulseInProgress(ctx, spaceID)
	if ps.Standup {
		resp["standup"] = a.pulseStandup(ctx, spaceID, u.ID)
	}
	writeJSON(w, http.StatusOK, resp)
}

// pulseNextAction picks the single most useful thing to do next, in priority order:
// unblock > assign > schedule > chase an overdue item. Only enabled signals are considered,
// so the suggestion never points at something the owner chose not to track.
func (a *API) pulseNextAction(ctx context.Context, spaceID int64, ps pulseSettings) *nextAction {
	type probe struct {
		kind string
		on   bool
		sql  string
	}
	base := `SELECT t.id, t.title FROM tasks t JOIN lists l ON l.id=t.list_id
		WHERE l.space_id=$1 AND t.archived_at IS NULL AND l.archived_at IS NULL AND t.completed_at IS NULL `
	probes := []probe{
		{"unblock", ps.Signals["blocked"], base + `AND COALESCE(array_length(t.blocked_by,1),0) > 0
			ORDER BY t.due_at NULLS LAST, t.id LIMIT 1`},
		{"assign", ps.Signals["unassigned"], base + `AND t.assignee_id IS NULL
			ORDER BY t.due_at NULLS LAST, t.id LIMIT 1`},
		{"schedule", ps.Signals["no_deadline"], base + `AND t.due_at IS NULL
			ORDER BY t.updated_at, t.id LIMIT 1`},
		{"overdue", ps.Signals["overdue"], base + `AND t.due_at < now()
			ORDER BY t.due_at, t.id LIMIT 1`},
	}
	for _, p := range probes {
		if !p.on {
			continue
		}
		var act nextAction
		if a.DB.Pool.QueryRow(ctx, p.sql, spaceID).Scan(&act.TaskID, &act.Title) == nil {
			act.Kind = p.kind
			return &act
		}
	}
	return nil
}

type pulseTask struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	Assignee *string `json:"assignee"`
	Progress *int    `json:"progress"`
}

// pulseInProgress is the "Сейчас в работе" block from the spec mockup.
func (a *API) pulseInProgress(ctx context.Context, spaceID int64) []pulseTask {
	out := []pulseTask{}
	rows, err := a.DB.Pool.Query(ctx, `
		SELECT t.id, t.title, u.username, t.progress
		FROM tasks t
		JOIN lists l ON l.id = t.list_id
		LEFT JOIN users u ON u.id = t.assignee_id
		WHERE l.space_id=$1 AND t.archived_at IS NULL AND l.archived_at IS NULL
		  AND t.completed_at IS NULL AND t.status='in_progress'
		ORDER BY t.updated_at DESC LIMIT 5`, spaceID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var t pulseTask
		if rows.Scan(&t.ID, &t.Title, &t.Assignee, &t.Progress) == nil {
			out = append(out, t)
		}
	}
	return out
}

// pulseStandup is the daily mini-standup: what the viewer closed in the last 24h, what they're
// on now, and what's blocking them. Scoped to the current user — a standup is personal.
func (a *API) pulseStandup(ctx context.Context, spaceID, userID int64) map[string]any {
	since := time.Now().Add(-24 * time.Hour)
	collect := func(sql string, args ...any) []pulseTask {
		out := []pulseTask{}
		rows, err := a.DB.Pool.Query(ctx, sql, args...)
		if err != nil {
			return out
		}
		defer rows.Close()
		for rows.Next() {
			var t pulseTask
			if rows.Scan(&t.ID, &t.Title) == nil {
				out = append(out, t)
			}
		}
		return out
	}
	base := `SELECT t.id, t.title FROM tasks t JOIN lists l ON l.id=t.list_id
		WHERE l.space_id=$1 AND t.archived_at IS NULL AND l.archived_at IS NULL AND t.assignee_id=$2 `
	return map[string]any{
		"did": collect(base+`AND t.completed_at >= $3 ORDER BY t.completed_at DESC LIMIT 5`,
			spaceID, userID, since),
		"doing": collect(base+`AND t.completed_at IS NULL AND t.status='in_progress'
			ORDER BY t.updated_at DESC LIMIT 5`, spaceID, userID),
		"blocked": collect(base+`AND t.completed_at IS NULL AND COALESCE(array_length(t.blocked_by,1),0) > 0
			ORDER BY t.due_at NULLS LAST LIMIT 5`, spaceID, userID),
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
