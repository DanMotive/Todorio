package api

// Recurring tasks: when a task with a recurrence rule is marked done, the next occurrence is
// created automatically (same list/title/assignee/weight), due_at advanced by the rule.
// Rule format (tasks.recurrence JSONB): {"freq": "daily"|"weekly"|"monthly", "interval": 1}

import (
	"context"
	"encoding/json"
	"time"
)

type recurrenceRule struct {
	Freq     string `json:"freq"`
	Interval int    `json:"interval"`
}

func nextOccurrence(due time.Time, rule recurrenceRule) time.Time {
	n := rule.Interval
	if n < 1 {
		n = 1
	}
	switch rule.Freq {
	case "daily":
		return due.AddDate(0, 0, n)
	case "weekly":
		return due.AddDate(0, 0, 7*n)
	case "monthly":
		return due.AddDate(0, n, 0)
	default:
		return due.AddDate(0, 0, n)
	}
}

// spawnRecurrence creates the next occurrence of a completed recurring task. Best-effort:
// errors are ignored since this runs as a side effect of a successful task update.
func (a *API) spawnRecurrence(ctx context.Context, taskID int64) {
	var rawRecurrence *string
	var listID, creatorID int64
	var assigneeID *int64
	var title, description, priority string
	var dueAt *time.Time
	var weight int
	err := a.DB.Pool.QueryRow(ctx, `
		SELECT recurrence::text, list_id, creator_id, assignee_id, title, description, priority, due_at, weight
		FROM tasks WHERE id=$1`, taskID).
		Scan(&rawRecurrence, &listID, &creatorID, &assigneeID, &title, &description, &priority, &dueAt, &weight)
	if err != nil || rawRecurrence == nil {
		return
	}
	var rule recurrenceRule
	if json.Unmarshal([]byte(*rawRecurrence), &rule) != nil || rule.Freq == "" {
		return
	}
	base := time.Now()
	if dueAt != nil {
		base = *dueAt
	}
	newDue := nextOccurrence(base, rule)
	_, _ = a.DB.Pool.Exec(ctx, `
		INSERT INTO tasks(list_id, title, description, priority, assignee_id, due_at, weight, creator_id, recurrence)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)`,
		listID, title, description, priority, assigneeID, newDue, weight, creatorID, *rawRecurrence)
}
