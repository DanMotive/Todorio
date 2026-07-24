// Package worker: background jobs — deadline reminders, archive cleanup, stale sessions.
package worker

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DanMotive/Todorio/internal/db"
	"github.com/DanMotive/Todorio/internal/events"
)

func Run(ctx context.Context, d *db.DB, bus *events.Bus) {
	hourly := time.NewTicker(time.Hour)
	daily := time.NewTicker(24 * time.Hour)
	defer hourly.Stop()
	defer daily.Stop()

	// run once immediately on startup
	reminderSweep(ctx, d, bus)

	for {
		select {
		case <-ctx.Done():
			return
		case <-hourly.C:
			reminderSweep(ctx, d, bus)
		case <-daily.C:
			cleanupArchive(ctx, d)
			cleanupSessions(ctx, d)
		}
	}
}

// reminderPrefs mirrors users.notify_prefs.reminders (spec section 8): how many days before a
// deadline to warn, whether to warn on the due day itself, and whether overdue tasks keep getting
// a daily nudge. Any field left unset by the user falls back to a sensible default.
type reminderPrefs struct {
	BeforeDays   []int `json:"before_days"`
	OnDueDay     *bool `json:"on_due_day"`
	DailyOverdue *bool `json:"daily_overdue"`
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// reminderSweep — per-user configurable deadline reminders. Runs hourly; each helper below is
// idempotent per calendar day (or per 24h for "overdue", matching the original behavior) so a
// user assigned to the same task doesn't get spammed on every tick.
func reminderSweep(ctx context.Context, d *db.DB, bus *events.Bus) {
	rows, err := d.Pool.Query(ctx, `
		SELECT id, notify_prefs #>> '{reminders}', COALESCE(notify_prefs #>> '{types,overdue}', 'true')
		FROM users WHERE status='active' AND archived_at IS NULL`)
	if err != nil {
		log.Printf("worker: reminderSweep: listing users: %v", err)
		return
	}
	type userPrefs struct {
		id            int64
		beforeDays    []int
		onDueDay      bool
		dailyOverdue  bool
		overdueTypeOn bool
	}
	var users []userPrefs
	for rows.Next() {
		var id int64
		var remindersRaw *string
		var overdueType string
		if rows.Scan(&id, &remindersRaw, &overdueType) != nil {
			continue
		}
		up := userPrefs{id: id, beforeDays: []int{3, 1}, onDueDay: true, dailyOverdue: true, overdueTypeOn: overdueType != "false"}
		if remindersRaw != nil {
			var p reminderPrefs
			if json.Unmarshal([]byte(*remindersRaw), &p) == nil {
				if p.BeforeDays != nil {
					up.beforeDays = p.BeforeDays
				}
				up.onDueDay = boolOr(p.OnDueDay, true)
				up.dailyOverdue = boolOr(p.DailyOverdue, true)
			}
		}
		users = append(users, up)
	}
	rows.Close()

	for _, u := range users {
		for _, n := range u.beforeDays {
			if n > 0 {
				remindDueWithinDays(ctx, d, bus, u.id, n)
			}
		}
		if u.onDueDay {
			remindDueToday(ctx, d, bus, u.id)
		}
		if u.dailyOverdue && u.overdueTypeOn {
			remindOverdue(ctx, d, bus, u.id)
		}
	}
}

// remindDueWithinDays notifies userID, once per day, about their tasks due in exactly `days` days.
func remindDueWithinDays(ctx context.Context, d *db.DB, bus *events.Bus, userID int64, days int) {
	rows, err := d.Pool.Query(ctx, `
		SELECT t.id, t.title FROM tasks t
		WHERE t.assignee_id=$1 AND t.archived_at IS NULL AND t.completed_at IS NULL
			AND t.due_at IS NOT NULL AND t.due_at::date = CURRENT_DATE + $2::int
			AND NOT EXISTS (
				SELECT 1 FROM notifications n
				WHERE n.user_id=$1 AND n.kind='due_soon' AND n.payload->>'task_id' = t.id::text
					AND (n.payload->>'days')::int = $2
					AND n.created_at::date = CURRENT_DATE)`,
		userID, days)
	if err != nil {
		log.Printf("worker: remindDueWithinDays(%d): %v", days, err)
		return
	}
	notifyRows(ctx, d, bus, rows, userID, "due_soon", map[string]any{"days": days})
}

// remindDueToday notifies userID, once per day, about their tasks due today.
func remindDueToday(ctx context.Context, d *db.DB, bus *events.Bus, userID int64) {
	rows, err := d.Pool.Query(ctx, `
		SELECT t.id, t.title FROM tasks t
		WHERE t.assignee_id=$1 AND t.archived_at IS NULL AND t.completed_at IS NULL
			AND t.due_at IS NOT NULL AND t.due_at::date = CURRENT_DATE
			AND NOT EXISTS (
				SELECT 1 FROM notifications n
				WHERE n.user_id=$1 AND n.kind='due_today' AND n.payload->>'task_id' = t.id::text
					AND n.created_at::date = CURRENT_DATE)`,
		userID)
	if err != nil {
		log.Printf("worker: remindDueToday: %v", err)
		return
	}
	notifyRows(ctx, d, bus, rows, userID, "due_today", nil)
}

// remindOverdue — notifications for overdue tasks (at most once per ~24h per task); this is the
// original deadlineSweep behavior, now run per-user and gated by that user's own preference.
func remindOverdue(ctx context.Context, d *db.DB, bus *events.Bus, userID int64) {
	rows, err := d.Pool.Query(ctx, `
		SELECT t.id, t.title FROM tasks t
		WHERE t.assignee_id=$1 AND t.archived_at IS NULL AND t.completed_at IS NULL AND t.due_at < now()
			AND NOT EXISTS (
				SELECT 1 FROM notifications n
				WHERE n.user_id=$1 AND n.kind = 'overdue'
					AND n.payload->>'task_id' = t.id::text
					AND n.created_at > now() - interval '24 hours')`,
		userID)
	if err != nil {
		log.Printf("worker: remindOverdue: %v", err)
		return
	}
	notifyRows(ctx, d, bus, rows, userID, "overdue", nil)
}

// notifyRows drains a (task id, title) result set and creates+publishes one notification of
// `kind` per row, merging `extra` fields into each payload (and always closes rows).
func notifyRows(ctx context.Context, d *db.DB, bus *events.Bus, rows pgx.Rows, userID int64, kind string, extra map[string]any) {
	defer rows.Close()
	type item struct {
		id    int64
		title string
	}
	var items []item
	for rows.Next() {
		var it item
		if rows.Scan(&it.id, &it.title) == nil {
			items = append(items, it)
		}
	}
	for _, it := range items {
		payload := map[string]any{"task_id": it.id, "title": it.title}
		for k, v := range extra {
			payload[k] = v
		}
		b, _ := json.Marshal(payload)
		_, _ = d.Pool.Exec(ctx, `INSERT INTO notifications(user_id, kind, payload) VALUES($1,$2,$3)`, userID, kind, string(b))
		bus.Publish([]int64{userID}, events.Event{Type: "notification", Data: map[string]any{"kind": kind, "payload": payload}})
	}
}

// cleanupArchive — per spec, the archive self-cleans after 30 days (configurable via policy.archive.retention_days).
func cleanupArchive(ctx context.Context, d *db.DB) {
	days := d.Setting(ctx, "policy.archive.retention_days", "30")
	_, err := d.Pool.Exec(ctx, `
		DELETE FROM tasks WHERE archived_at IS NOT NULL AND archived_at < now() - ($1 || ' days')::interval`, days)
	if err != nil {
		log.Printf("worker: cleanupArchive: %v", err)
	}
	_, _ = d.Pool.Exec(ctx, `
		DELETE FROM lists WHERE archived_at IS NOT NULL AND archived_at < now() - ($1 || ' days')::interval`, days)
	_, _ = d.Pool.Exec(ctx, `
		DELETE FROM spaces WHERE archived_at IS NOT NULL AND archived_at < now() - ($1 || ' days')::interval`, days)
}

func cleanupSessions(ctx context.Context, d *db.DB) {
	_, _ = d.Pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
}
