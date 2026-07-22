// Package worker: background jobs — deadlines, archive cleanup, stale sessions.
package worker

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/DanMotive/Todorio/internal/db"
	"github.com/DanMotive/Todorio/internal/events"
)

func Run(ctx context.Context, d *db.DB, bus *events.Bus) {
	hourly := time.NewTicker(time.Hour)
	daily := time.NewTicker(24 * time.Hour)
	defer hourly.Stop()
	defer daily.Stop()

	// run once immediately on startup
	deadlineSweep(ctx, d, bus)

	for {
		select {
		case <-ctx.Done():
			return
		case <-hourly.C:
			deadlineSweep(ctx, d, bus)
		case <-daily.C:
			cleanupArchive(ctx, d)
			cleanupSessions(ctx, d)
		}
	}
}

// deadlineSweep — notifications for overdue tasks (at most once per day per task).
func deadlineSweep(ctx context.Context, d *db.DB, bus *events.Bus) {
	rows, err := d.Pool.Query(ctx, `
		SELECT t.id, t.title, t.assignee_id
		FROM tasks t
		WHERE t.archived_at IS NULL AND t.completed_at IS NULL
			AND t.assignee_id IS NOT NULL AND t.due_at < now()
			AND NOT EXISTS (
				SELECT 1 FROM notifications n
				WHERE n.user_id = t.assignee_id AND n.kind = 'overdue'
					AND n.payload->>'task_id' = t.id::text
					AND n.created_at > now() - interval '24 hours')`)
	if err != nil {
		log.Printf("worker: deadlineSweep: %v", err)
		return
	}
	defer rows.Close()
	type item struct {
		id       int64
		title    string
		assignee int64
	}
	var items []item
	for rows.Next() {
		var it item
		if rows.Scan(&it.id, &it.title, &it.assignee) == nil {
			items = append(items, it)
		}
	}
	for _, it := range items {
		payload, _ := json.Marshal(map[string]any{"task_id": it.id, "title": it.title})
		_, _ = d.Pool.Exec(ctx, `INSERT INTO notifications(user_id, kind, payload) VALUES($1,'overdue',$2)`,
			it.assignee, string(payload))
		bus.Publish([]int64{it.assignee}, events.Event{Type: "notification", Data: map[string]any{
			"kind": "overdue", "payload": map[string]any{"task_id": it.id, "title": it.title},
		}})
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
