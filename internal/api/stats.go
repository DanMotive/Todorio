package api

import (
	"context"
	"net/http"
	"strings"
)

// userLocale resolves the caller's own locale preference for locale-dependent content the
// server itself picks (stat captions, Telegram notification text) — deliberately the viewer's
// own profile language, never a single site-wide value, so what one person sees never depends
// on what language someone else (or root) happened to set.
func (a *API) userLocale(ctx context.Context, userID int64) string {
	var locale *string
	_ = a.DB.Pool.QueryRow(ctx, `SELECT locale FROM users WHERE id=$1`, userID).Scan(&locale)
	raw := ""
	if locale != nil {
		raw = *locale
	}
	return a.normalizeLocale(raw)
}

// normalizeLocale maps a raw stored locale value (possibly empty, possibly an "-it" tone
// overlay, possibly something no longer in allLocales) down to one of the 13 base locales that
// server-generated content (stat_captions, notif.* strings) is actually available in.
//
// "-it" tone overlays (web/src/i18n.ts) are a UI-string slang variant of the same base
// language, not a separate language those rows were ever generated for — resolve to the base
// locale underneath one.
func (a *API) normalizeLocale(raw string) string {
	if raw == "" {
		raw = a.Cfg.DefaultLocale
	}
	raw = strings.TrimSuffix(raw, "-it")
	for _, l := range allLocales {
		if l == raw {
			return raw
		}
	}
	return "en-US"
}

// captionCategory picks which set of phrases describes the period.
//
// Migration 0010 shipped seven categories in all 13 languages, but this function only ever
// returned four of them: perfect_day, inactive and project were unreachable, so 312 of the 728
// phrases could never appear. The rules below make each one reachable, and each one says
// something the others do not:
//
//   - overdue     — more was missed than finished. Nothing else matters if that is true.
//   - perfect_day — work was done, nothing is overdue, and the board is empty. This is the
//     literal claim the phrases make ("Zero leftovers", "Nothing left behind"), so it requires
//     an actually empty board rather than merely a good week.
//   - inactive    — nothing was completed and nothing is overdue: a quiet period, not a failure.
//   - success     — high output, work still open.
//   - project     — steady progress that visibly outpaces what is left.
//   - focus       — some progress, but the backlog still dominates.
//   - neutral     — one or two tasks; too little to characterise.
//
// Pure function of three counters, so the thresholds are testable without a database.
func captionCategory(done, overdue, open int) string {
	switch {
	case overdue > 0 && overdue > done:
		return "overdue"
	case done > 0 && overdue == 0 && open == 0:
		return "perfect_day"
	case done == 0 && overdue == 0:
		return "inactive"
	case done >= 10:
		return "success"
	case done >= 3 && done >= open:
		return "project"
	case done >= 3:
		return "focus"
	default:
		return "neutral"
	}
}

// pickCaption returns one half of the caption, falling back rather than returning nothing.
//
// Previously a missing row meant a silently blank caption — the failure mode that migration
// 0010 was written to fix, and one that would come back the moment a new category or a new
// locale is added without a full set of phrases. The order is: the viewer's language, then
// English for the same category, then the viewer's language with the always-populated neutral
// set. Empty only if stat_captions itself is empty.
func (a *API) pickCaption(ctx context.Context, locale, category string, part int, seed int64) string {
	attempts := [][2]string{
		{locale, category},
		{"en-US", category},
		{locale, "neutral"},
		{"en-US", "neutral"},
	}
	for _, attempt := range attempts {
		var text string
		err := a.DB.Pool.QueryRow(ctx, `
			SELECT text FROM stat_captions WHERE locale=$1 AND category=$2 AND part=$3
			ORDER BY id OFFSET (($4 + EXTRACT(DOY FROM now())::int * $3) % GREATEST(
				(SELECT count(*) FROM stat_captions WHERE locale=$1 AND category=$2 AND part=$3), 1)) LIMIT 1`,
			attempt[0], attempt[1], part, seed).Scan(&text)
		if err == nil && text != "" {
			return text
		}
	}
	return ""
}

// GET /api/spaces/{id}/stats?period=week|month — space statistics:
// per-member (done, weighted contribution), "top performer", and a random two-part caption.
// Rankings can be disabled: spaces.settings -> stats.show_best = false.
func (a *API) handleStats(w http.ResponseWriter, r *http.Request) {
	u := a.requireUser(w, r)
	if u == nil {
		return
	}
	spaceID, err := pathID(r)
	if err != nil || a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "" {
		errJSON(w, http.StatusForbidden, "no access to the space")
		return
	}
	if !a.featureEnabled(r.Context(), "stats") {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	interval := "7 days"
	if r.URL.Query().Get("period") == "month" {
		interval = "30 days"
	}

	rows, err := a.DB.Pool.Query(r.Context(), `
		SELECT u.id, u.username, COALESCE(u.display_name, u.username),
			count(t.id) FILTER (WHERE t.completed_at > now() - $2::interval)::int AS done_cnt,
			COALESCE(sum(t.weight) FILTER (WHERE t.completed_at > now() - $2::interval), 0)::int AS done_weight,
			count(t.id) FILTER (WHERE t.completed_at IS NULL AND t.due_at < now())::int AS overdue_cnt
		FROM space_members m
		JOIN users u ON u.id = m.user_id AND u.status = 'active'
		LEFT JOIN tasks t ON t.assignee_id = u.id AND t.archived_at IS NULL
			AND t.list_id IN (SELECT id FROM lists WHERE space_id = $1 AND archived_at IS NULL)
		WHERE m.space_id = $1
		GROUP BY u.id, u.username, u.display_name
		ORDER BY done_weight DESC, done_cnt DESC`, spaceID, interval)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	type member struct {
		ID         int64  `json:"id"`
		Username   string `json:"username"`
		Name       string `json:"name"`
		Done       int    `json:"done"`
		DoneWeight int    `json:"done_weight"`
		Overdue    int    `json:"overdue"`
	}
	members := []member{}
	totalDone, totalOverdue := 0, 0
	for rows.Next() {
		var m member
		if rows.Scan(&m.ID, &m.Username, &m.Name, &m.Done, &m.DoneWeight, &m.Overdue) == nil {
			members = append(members, m)
			totalDone += m.Done
			totalOverdue += m.Overdue
		}
	}

	// How much work is still open in the space. The per-member totals alone cannot tell
	// "finished everything" apart from "finished a bit, plenty left", which is exactly the
	// distinction the perfect_day and project captions are written for.
	openRemaining := 0
	_ = a.DB.Pool.QueryRow(r.Context(), `
		SELECT count(*)::int FROM tasks
		WHERE archived_at IS NULL AND completed_at IS NULL
			AND list_id IN (SELECT id FROM lists WHERE space_id = $1 AND archived_at IS NULL)`,
		spaceID).Scan(&openRemaining)

	category := captionCategory(totalDone, totalOverdue, openRemaining)

	// Two-part caption: deterministic "random" pick per day (seed = space_id + day of year),
	// so the caption doesn't change on every page refresh. The two halves use different seeds
	// so they don't move in lockstep.
	locale := a.userLocale(r.Context(), u.ID)
	part1 := a.pickCaption(r.Context(), locale, category, 1, spaceID)
	part2 := a.pickCaption(r.Context(), locale, category, 2, spaceID*7)

	// Leaderboard visibility (spec section 14: "кому показывать — своё место / топ-3 / полная
	// таблица / только владельцу"). Read from spaces.settings->stats->visibility; anything
	// unset or unrecognised keeps the previous behaviour of showing the full table.
	//
	// This is filtered server-side, not just hidden in the UI: "only the owner may see the
	// table" has to mean the rows never reach anyone else's browser.
	visibility := "full"
	var visRaw *string
	_ = a.DB.Pool.QueryRow(r.Context(),
		`SELECT settings #>> '{stats,visibility}' FROM spaces WHERE id=$1`, spaceID).Scan(&visRaw)
	if visRaw != nil {
		switch *visRaw {
		case "own", "top3", "full", "owner_only":
			visibility = *visRaw
		}
	}
	isOwner := a.spaceRole(r, u.ID, u.IsAdmin(), spaceID) == "owner"

	// The unfiltered ranking is needed to work out the viewer's own position before trimming.
	myRank := 0
	for i, m := range members {
		if m.ID == u.ID {
			myRank = i + 1
			break
		}
	}

	shown := members
	switch visibility {
	case "owner_only":
		if !isOwner {
			shown = []member{}
		}
	case "top3":
		if !isOwner && len(shown) > 3 {
			shown = shown[:3]
		}
	case "own":
		if !isOwner {
			shown = []member{}
			for _, m := range members {
				if m.ID == u.ID {
					shown = append(shown, m)
					break
				}
			}
		}
	}

	resp := map[string]any{
		"period":       interval,
		"members":      shown,
		"visibility":   visibility,
		"my_rank":      myRank,
		"total_ranked": len(members),
		"caption":      map[string]string{"part1": part1, "part2": part2, "category": category},
	}

	// "Top performer" — configurable by the space owner (settings.stats.show_best). Suppressed
	// when the leaderboard is owner-only, since naming the top performer would leak exactly
	// what that setting is meant to keep private.
	var showBest *bool
	_ = a.DB.Pool.QueryRow(r.Context(),
		`SELECT (settings #>> '{stats,show_best}')::boolean FROM spaces WHERE id=$1`, spaceID).Scan(&showBest)
	if (showBest == nil || *showBest) && len(members) > 0 && members[0].DoneWeight > 0 &&
		(visibility != "owner_only" || isOwner) {
		resp["best"] = members[0]
	}

	writeJSON(w, http.StatusOK, resp)
}
