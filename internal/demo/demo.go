// Package demo: onboarding demo space and quests.
package demo

import (
	"context"
	"fmt"

	"github.com/DanMotive/Todorio/internal/db"
)

type Quest struct {
	Title       string
	Description string
}

// Quests — tasks to help a new user learn the product (a guided tour as to-dos).
func Quests() []Quest {
	return []Quest{
		{"✅ Close this task", "Click the circle to the left of the title — that's how completed tasks are marked."},
		{"📝 Create your first task", "Open your list and click \"New task\". Name it anything you like!"},
		{"⏰ Set a due date", "Open a task and set a due date — a colored badge will appear, and you'll get a notification before it's overdue."},
		{"💬 Write a comment", "Every task card has a discussion feed. Mention a teammate with @username and they'll get a notification."},
		{"🔥 Add a reaction", "Any task or comment can get an emoji reaction: 👍 ✅ 🎉 🔥 👀 ❓ ❗ ❌ 😭 ⭐"},
		{"🎨 Switch the theme", "The site header has a choice of 5 colors, a light/dark scheme, and a lite mode for slower machines."},
		{"📊 Check the space Pulse", "The space page shows team health: overdue tasks, stalled tasks, and an overall score."},
		{"📱 Install as an app", "The ⬇️ button in the header installs Todorio as a PWA on your phone or desktop (requires HTTPS)."},
	}
}

// EnsureDemo creates the demo space on first run (if enabled during setup).
// Safe to call again — it's a no-op after the first run (flag: onboarding.demo_space_id in system_settings).
func EnsureDemo(ctx context.Context, d *db.DB) error {
	if d.Setting(ctx, "onboarding.demo", "on") == "off" {
		return nil
	}
	if d.Setting(ctx, "onboarding.demo_space_id", "") != "" {
		return nil // already created
	}
	var rootID int64
	if err := d.Pool.QueryRow(ctx,
		`SELECT id FROM users WHERE role='root' ORDER BY id LIMIT 1`).Scan(&rootID); err != nil {
		return nil // root not created yet — try again on the next startup
	}

	var spaceID int64
	if err := d.Pool.QueryRow(ctx,
		`INSERT INTO spaces(name, owner_id) VALUES($1,$2) RETURNING id`,
		"🎓 Welcome to Todorio", rootID).Scan(&spaceID); err != nil {
		return err
	}
	if _, err := d.Pool.Exec(ctx,
		`INSERT INTO space_members(space_id,user_id,role) VALUES($1,$2,'owner')`, spaceID, rootID); err != nil {
		return err
	}
	var listID int64
	if err := d.Pool.QueryRow(ctx,
		`INSERT INTO lists(space_id, name) VALUES($1,$2) RETURNING id`,
		spaceID, "🎯 Onboarding quests").Scan(&listID); err != nil {
		return err
	}
	if _, err := d.Pool.Exec(ctx,
		`INSERT INTO list_members(list_id,user_id,permission) VALUES($1,$2,'owner')`, listID, rootID); err != nil {
		return err
	}
	for _, q := range Quests() {
		if _, err := d.Pool.Exec(ctx, `
			INSERT INTO tasks(list_id, title, description, priority, assignee_id, creator_id)
			VALUES($1,$2,$3,'normal',$4,$4)`, listID, q.Title, q.Description, rootID); err != nil {
			return err
		}
	}
	return d.SetSetting(ctx, "onboarding.demo_space_id", fmt.Sprintf(`"%d"`, spaceID))
}
