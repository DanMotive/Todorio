package ops

// `todorio testsql` — a temporary diagnostic command that executes every non-trivial SQL query
// in the product against the real database and reports which ones actually work.
//
// Why this exists: the development sandbox has no PostgreSQL, so queries could only ever be
// checked by compiling the Go around them. `go build` cannot catch a misspelled column, a
// wrong cast, or a broken JOIN — those only surface at runtime, on a real database, which is
// exactly the class of bug that shipped once already (tasks.position referenced by a query but
// never created by a migration; every list read 500'd).
//
// Safety: everything runs inside ONE transaction that is ALWAYS rolled back. Writes are
// exercised so INSERT/UPDATE statements are genuinely validated, but nothing is committed —
// running this against the production database does not change a single row. Sequences may
// advance (Postgres does not roll those back), which is harmless.
//
// This command is meant to be removed once the schema has been verified in the field.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DanMotive/Todorio/internal/config"
	"github.com/DanMotive/Todorio/internal/db"
	"github.com/DanMotive/Todorio/internal/term"
	"github.com/jackc/pgx/v5"
)

type sqlCheck struct {
	name string
	// run executes the check; returning an error marks it failed.
	run func(ctx context.Context, tx pgx.Tx, f *fixture) error
}

// fixture holds ids created at the start of the transaction so query checks have real rows to
// work against — an empty database would let a broken query "pass" by matching nothing.
type fixture struct {
	userID, spaceID, listID, taskID, subtaskID, commentID int64
	username                                              string
}

func TestSQL(cfg config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println()
	fmt.Println(term.Bold("Todorio — SQL self-test"))
	fmt.Println("  All statements run in one transaction and are rolled back at the end.")
	fmt.Println("  Your data will not be modified.")
	fmt.Println(strings.Repeat("─", 62))

	database, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to the database: %w", err)
	}
	defer database.Pool.Close()

	// --- schema sanity, outside the transaction (read-only) ---
	fmt.Println(term.Bold("Schema"))
	schemaOK := true
	for _, c := range schemaChecks() {
		var exists bool
		if err := database.Pool.QueryRow(ctx, c.query).Scan(&exists); err != nil || !exists {
			bad(c.label)
			schemaOK = false
		} else {
			ok(c.label)
		}
	}
	if !schemaOK {
		fmt.Println()
		warn("Schema is incomplete — run the server once so migrations apply, then retry.")
	}

	// --- applied migrations ---
	fmt.Println()
	fmt.Println(term.Bold("Migrations"))
	rows, err := database.Pool.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		bad("schema_migrations is unreadable: " + err.Error())
	} else {
		var applied []string
		for rows.Next() {
			var v string
			if rows.Scan(&v) == nil {
				applied = append(applied, v)
			}
		}
		rows.Close()
		ok(fmt.Sprintf("%d applied: %s", len(applied), strings.Join(applied, ", ")))
	}

	// --- query checks, all inside a rolled-back transaction ---
	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("starting the test transaction: %w", err)
	}
	// Rollback is deferred immediately so any panic or early return still leaves the DB clean.
	defer func() { _ = tx.Rollback(ctx) }()

	fmt.Println()
	fmt.Println(term.Bold("Fixtures"))
	f, err := seedFixture(ctx, tx)
	if err != nil {
		bad("could not create test rows: " + err.Error())
		fmt.Println()
		warn("Query checks skipped. The error above is the real problem — it means an INSERT")
		warn("used by the app does not match the schema.")
		return nil
	}
	ok(fmt.Sprintf("temporary user/space/list/task created (task id %d)", f.taskID))

	fmt.Println()
	fmt.Println(term.Bold("Queries"))
	var passed, failed int
	for _, c := range allChecks() {
		// Each check runs in a savepoint: one failure must not poison the transaction and
		// abort every check after it.
		if _, err := tx.Exec(ctx, "SAVEPOINT sp"); err != nil {
			bad("savepoint: " + err.Error())
			failed++
			continue
		}
		if err := c.run(ctx, tx, f); err != nil {
			bad(c.name + " — " + firstLine(err.Error()))
			failed++
			_, _ = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT sp")
		} else {
			ok(c.name)
			passed++
			_, _ = tx.Exec(ctx, "RELEASE SAVEPOINT sp")
		}
	}

	_ = tx.Rollback(ctx)

	fmt.Println()
	fmt.Println(strings.Repeat("─", 62))
	if failed == 0 {
		fmt.Printf("  %s  %d/%d queries OK — everything was rolled back.\n",
			term.Green("[PASS]"), passed, passed)
	} else {
		fmt.Printf("  %s  %d passed, %s — everything was rolled back.\n",
			term.Yellow("[PARTIAL]"), passed, term.Red(fmt.Sprintf("%d failed", failed)))
		fmt.Println("  Send the failing lines above to the developer.")
	}
	fmt.Println()
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

type schemaCheck struct {
	label string
	query string
}

// schemaChecks verifies the columns added by later migrations actually exist — the exact class
// of failure that shipped before (a query referencing a column no migration created).
func schemaChecks() []schemaCheck {
	col := func(table, column string) schemaCheck {
		return schemaCheck{
			label: fmt.Sprintf("%s.%s", table, column),
			query: fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM information_schema.columns
				WHERE table_name='%s' AND column_name='%s')`, table, column),
		}
	}
	return []schemaCheck{
		col("tasks", "position"),    // 0005
		col("tasks", "archived_by"), // 0006
		col("tasks", "start_at"),    // 0007 — Timeline
		col("tasks", "weight"),      // weighted progress
		col("tasks", "progress"),    // manual progress slider
		col("tasks", "blocked_by"),  // dependencies
		col("attachments", "target_type"),
		col("templates", "audience"), // template visibility
		col("spaces", "settings"),    // Pulse / stats / fields config
		col("comments", "deleted_at"),
		{
			label: "users.theme_scheme is gone (0008)",
			query: `SELECT NOT EXISTS(SELECT 1 FROM information_schema.columns
				WHERE table_name='users' AND column_name='theme_scheme')`,
		},
		col("tasks", "review_state"), // 0009 — review workflow
		col("tasks", "review_note"),
		col("comments", "parent_id"), // 0009 — threaded replies
		{
			label: "task_watchers table exists (0009)",
			query: `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='task_watchers')`,
		},
		{
			label: "action_counters table exists (0009)",
			query: `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='action_counters')`,
		},
		{
			label: "tasks_schedule_idx exists (0007)",
			query: `SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname='tasks_schedule_idx')`,
		},
		col("users", "telegram_chat_id"), // 0011 — Telegram notification linking
		col("users", "telegram_link_code"),
		{
			label: "users_telegram_chat_id_idx exists (0011)",
			query: `SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname='users_telegram_chat_id_idx')`,
		},
	}
}

// seedFixture creates a disposable user/space/list/task graph. It doubles as a write test:
// these are the same INSERT shapes the API uses.
func seedFixture(ctx context.Context, tx pgx.Tx) (*fixture, error) {
	f := &fixture{username: fmt.Sprintf("__sqltest_%d", time.Now().UnixNano()%1e9)}

	if err := tx.QueryRow(ctx,
		`INSERT INTO users(username, password_hash, role, status) VALUES($1,'x','user','active') RETURNING id`,
		f.username).Scan(&f.userID); err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO spaces(name, owner_id) VALUES($1,$2) RETURNING id`,
		"__sqltest space", f.userID).Scan(&f.spaceID); err != nil {
		return nil, fmt.Errorf("insert space: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO space_members(space_id,user_id,role) VALUES($1,$2,'owner')`, f.spaceID, f.userID); err != nil {
		return nil, fmt.Errorf("insert space_member: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO lists(space_id, name) VALUES($1,$2) RETURNING id`,
		f.spaceID, "__sqltest list").Scan(&f.listID); err != nil {
		return nil, fmt.Errorf("insert list: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO list_members(list_id,user_id,permission) VALUES($1,$2,'owner')`, f.listID, f.userID); err != nil {
		return nil, fmt.Errorf("insert list_member: %w", err)
	}
	// Exactly the INSERT handleCreateTask runs, including start_at (0007).
	if err := tx.QueryRow(ctx, `
		INSERT INTO tasks(list_id, parent_id, title, description, priority, assignee_id, start_at, due_at, weight, creator_id)
		VALUES($1,$2,$3,$4,COALESCE($5,'normal'),$6,$7,$8,COALESCE($9,1),$10) RETURNING id`,
		f.listID, nil, "__sqltest task", "", nil, f.userID,
		time.Now().Add(-24*time.Hour), time.Now().Add(48*time.Hour), 3, f.userID).Scan(&f.taskID); err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO tasks(list_id, parent_id, title, description, creator_id)
		VALUES($1,$2,$3,'',$4) RETURNING id`,
		f.listID, f.taskID, "__sqltest subtask", f.userID).Scan(&f.subtaskID); err != nil {
		return nil, fmt.Errorf("insert subtask: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO comments(task_id, author_id, body) VALUES($1,$2,$3) RETURNING id`,
		f.taskID, f.userID, "hello @"+f.username).Scan(&f.commentID); err != nil {
		return nil, fmt.Errorf("insert comment: %w", err)
	}
	return f, nil
}

// allChecks exercises the real queries from the API, grouped by the feature they belong to.
func allChecks() []sqlCheck {
	// exec runs a statement and only cares that Postgres accepted it.
	exec := func(name, sql string, args func(f *fixture) []any) sqlCheck {
		return sqlCheck{name: name, run: func(ctx context.Context, tx pgx.Tx, f *fixture) error {
			_, err := tx.Exec(ctx, sql, args(f)...)
			return err
		}}
	}
	// query runs a SELECT and drains it, so column/type errors surface.
	query := func(name, sql string, args func(f *fixture) []any) sqlCheck {
		return sqlCheck{name: name, run: func(ctx context.Context, tx pgx.Tx, f *fixture) error {
			rows, err := tx.Query(ctx, sql, args(f)...)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
			}
			return rows.Err()
		}}
	}

	return []sqlCheck{
		// --- tasks ---
		query("task list read (taskSelect)", taskSelectSQL+` WHERE t.list_id=$1 AND t.archived_at IS NULL ORDER BY t.position, t.id`,
			func(f *fixture) []any { return []any{f.listID} }),
		query("my tasks", taskSelectSQL+` WHERE t.assignee_id=$1 AND t.archived_at IS NULL AND t.completed_at IS NULL
			ORDER BY t.due_at NULLS LAST, t.priority DESC, t.id`,
			func(f *fixture) []any { return []any{f.userID} }),
		exec("task update (all fields + clear flags)", `
			UPDATE tasks SET
				title       = COALESCE($2, title),
				description = COALESCE($3, description),
				status      = COALESCE($4, status),
				priority    = COALESCE($5, priority),
				assignee_id = CASE WHEN $7 THEN NULL ELSE COALESCE($6, assignee_id) END,
				due_at      = CASE WHEN $9 THEN NULL ELSE COALESCE($8, due_at) END,
				start_at    = CASE WHEN $19 THEN NULL ELSE COALESCE($18, start_at) END,
				progress    = CASE WHEN $17 THEN NULL ELSE COALESCE($10, progress) END,
				weight      = COALESCE($11, weight),
				blocked_by  = COALESCE($12, blocked_by),
				position    = COALESCE($13, position),
				recurrence  = CASE WHEN $14 THEN NULL ELSE COALESCE($15, recurrence) END,
				custom_fields = COALESCE($16, custom_fields),
				completed_at = CASE
					WHEN $4 = 'done' AND completed_at IS NULL THEN now()
					WHEN $4 IS NOT NULL AND $4 <> 'done' THEN NULL
					ELSE completed_at END,
				updated_at  = now()
			WHERE id=$1`,
			func(f *fixture) []any {
				return []any{f.taskID, "renamed", nil, "in_progress", "high",
					f.userID, false, time.Now().Add(72 * time.Hour), false,
					50, 4, []int64{f.subtaskID}, 1, false, nil, nil, false,
					time.Now().Add(-12 * time.Hour), false}
			}),
		exec("quick-add labels merge", `UPDATE tasks SET custom_fields = custom_fields || $2::jsonb WHERE id=$1`,
			func(f *fixture) []any { return []any{f.taskID, `{"labels":"infra,urgent"}`} }),
		exec("task version snapshot", `
			INSERT INTO task_versions(task_id, editor_id, snapshot)
			SELECT id, $2, to_jsonb(t) FROM tasks t WHERE id=$1`,
			func(f *fixture) []any { return []any{f.taskID, f.userID} }),

		// --- lists with weighted progress ---
		query("lists + weighted progress", `
			SELECT l.id, l.name, l.is_private, COALESCE(lm.permission,'') AS my_perm, l.position,
				(SELECT count(*) FROM tasks t WHERE t.list_id=l.id AND t.archived_at IS NULL),
				(SELECT count(*) FROM tasks t WHERE t.list_id=l.id AND t.archived_at IS NULL AND t.completed_at IS NOT NULL),
				(SELECT COALESCE(sum(t.weight),0) FROM tasks t WHERE t.list_id=l.id AND t.archived_at IS NULL),
				(SELECT COALESCE(sum(t.weight),0) FROM tasks t WHERE t.list_id=l.id AND t.archived_at IS NULL AND t.completed_at IS NOT NULL)
			FROM lists l
			LEFT JOIN list_members lm ON lm.list_id=l.id AND lm.user_id=$2
			WHERE l.space_id=$1 AND l.archived_at IS NULL
				AND ($3 OR lm.user_id IS NOT NULL OR l.is_private=false)
			ORDER BY l.position, l.id`,
			func(f *fixture) []any { return []any{f.spaceID, f.userID, false} }),

		// --- Space Pulse ---
		query("pulse counts (make_interval)", `
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
			func(f *fixture) []any { return []any{f.spaceID, 3} }),
		query("pulse settings read", `SELECT settings->'pulse' FROM spaces WHERE id=$1`,
			func(f *fixture) []any { return []any{f.spaceID} }),
		query("pulse in-progress", `
			SELECT t.id, t.title, u.username, t.progress
			FROM tasks t JOIN lists l ON l.id = t.list_id
			LEFT JOIN users u ON u.id = t.assignee_id
			WHERE l.space_id=$1 AND t.archived_at IS NULL AND l.archived_at IS NULL
			  AND t.completed_at IS NULL AND t.status='in_progress'
			ORDER BY t.updated_at DESC LIMIT 5`,
			func(f *fixture) []any { return []any{f.spaceID} }),
		exec("space settings shallow merge", `
			UPDATE spaces SET name = COALESCE($2, name),
				settings = settings || COALESCE($3, '{}'::jsonb)
			WHERE id=$1`,
			func(f *fixture) []any { return []any{f.spaceID, nil, `{"pulse":{"stale_days":5}}`} }),

		// --- Timeline (0007) ---
		query("timeline window", `
			SELECT t.id, t.list_id, l.name, t.parent_id, t.title, t.status, t.priority,
				usr.username, t.start_at, t.due_at, t.progress,
				(SELECT count(*) FROM tasks s WHERE s.parent_id=t.id AND s.archived_at IS NULL AND s.completed_at IS NOT NULL)::int,
				(SELECT count(*) FROM tasks s WHERE s.parent_id=t.id AND s.archived_at IS NULL)::int,
				COALESCE(t.blocked_by, '{}'), t.completed_at, lm.permission
			FROM tasks t
			JOIN lists l ON l.id = t.list_id
			LEFT JOIN list_members lm ON lm.list_id = l.id AND lm.user_id = $2
			LEFT JOIN users usr ON usr.id = t.assignee_id
			WHERE l.space_id = $1
			  AND t.archived_at IS NULL AND l.archived_at IS NULL
			  AND ($3 OR lm.user_id IS NOT NULL OR l.is_private = false)
			  AND ($4 = 0 OR t.list_id = $4)
			  AND (t.start_at IS NOT NULL OR t.due_at IS NOT NULL)
			  AND COALESCE(t.start_at, t.due_at) < $6
			  AND COALESCE(t.due_at, t.start_at) >= $5
			ORDER BY COALESCE(t.start_at, t.due_at), t.id`,
			func(f *fixture) []any {
				return []any{f.spaceID, f.userID, false, int64(0),
					time.Now().Add(-30 * 24 * time.Hour), time.Now().Add(90 * 24 * time.Hour)}
			}),
		query("timeline unscheduled count", `
			SELECT count(*)::int
			FROM tasks t
			JOIN lists l ON l.id = t.list_id
			LEFT JOIN list_members lm ON lm.list_id = l.id AND lm.user_id = $2
			WHERE l.space_id = $1
			  AND t.archived_at IS NULL AND l.archived_at IS NULL AND t.completed_at IS NULL
			  AND ($3 OR lm.user_id IS NOT NULL OR l.is_private = false)
			  AND ($4 = 0 OR t.list_id = $4)
			  AND t.start_at IS NULL AND t.due_at IS NULL`,
			func(f *fixture) []any { return []any{f.spaceID, f.userID, false, int64(0)} }),

		// --- Inbox ---
		query("inbox triage", `
			WITH visible AS (
				SELECT t.id, t.list_id, t.title, t.status, COALESCE(t.priority,'normal') AS priority,
						t.due_at, t.created_at,
					t.assignee_id, t.creator_id, l.name AS list_name, l.space_id, s.name AS space_name
				FROM tasks t
				JOIN lists l ON l.id = t.list_id
				JOIN spaces s ON s.id = l.space_id
				LEFT JOIN list_members lm ON lm.list_id = l.id AND lm.user_id = $1
				LEFT JOIN space_members sm ON sm.space_id = l.space_id AND sm.user_id = $1
				WHERE t.archived_at IS NULL AND l.archived_at IS NULL AND s.archived_at IS NULL
				  AND t.completed_at IS NULL
				  AND ($2 OR lm.user_id IS NOT NULL OR (l.is_private = false AND sm.user_id IS NOT NULL))
			)
			SELECT v.id, v.list_id, v.list_name, v.space_id, v.space_name, v.title, v.status,
				v.priority, v.due_at, v.created_at,
				CASE
					WHEN v.status = 'review' AND v.assignee_id = $1 THEN 'review'
					WHEN v.assignee_id = $1 AND v.due_at IS NULL     THEN 'assigned'
					WHEN v.assignee_id IS NULL AND v.creator_id = $1 THEN 'unassigned'
					ELSE 'mentioned'
				END AS reason
			FROM visible v
			WHERE (v.status = 'review' AND v.assignee_id = $1)
			   OR (v.assignee_id = $1 AND v.due_at IS NULL)
			   OR (v.assignee_id IS NULL AND v.creator_id = $1)
			   OR EXISTS (
			        SELECT 1 FROM comments c
			        WHERE c.task_id = v.id AND c.deleted_at IS NULL
			          AND c.body ILIKE '%@' || $3 || '%'
			          AND c.author_id <> $1
			          AND COALESCE(v.assignee_id, 0) <> $1
			      )
			ORDER BY v.created_at DESC
			LIMIT 200`,
			func(f *fixture) []any { return []any{f.userID, false, f.username} }),

		// --- stats and leaderboard ---
		query("space stats + leaderboard", `
			SELECT u.id, u.username, COALESCE(u.display_name, u.username),
				count(t.id) FILTER (WHERE t.completed_at > now() - $2::interval)::int,
				COALESCE(sum(t.weight) FILTER (WHERE t.completed_at > now() - $2::interval), 0)::int,
				count(t.id) FILTER (WHERE t.completed_at IS NULL AND t.due_at < now())::int
			FROM space_members m
			JOIN users u ON u.id = m.user_id AND u.status = 'active'
			LEFT JOIN tasks t ON t.assignee_id = u.id AND t.archived_at IS NULL
				AND t.list_id IN (SELECT id FROM lists WHERE space_id = $1 AND archived_at IS NULL)
			WHERE m.space_id = $1
			GROUP BY u.id, u.username, u.display_name
			ORDER BY 5 DESC, 4 DESC`,
			func(f *fixture) []any { return []any{f.spaceID, "7 days"} }),
		query("leaderboard visibility setting", `SELECT settings #>> '{stats,visibility}' FROM spaces WHERE id=$1`,
			func(f *fixture) []any { return []any{f.spaceID} }),
		query("stat_captions pick", `
			SELECT text FROM stat_captions WHERE locale=$1 AND category=$2 AND part=1
			ORDER BY id OFFSET (($3 + EXTRACT(DOY FROM now())::int) % GREATEST(
				(SELECT count(*) FROM stat_captions WHERE locale=$1 AND category=$2 AND part=1), 1)) LIMIT 1`,
			func(f *fixture) []any { return []any{"en-US", "success", f.spaceID} }),

		// --- attachments (task + comment) ---
		exec("attachment insert (comment target)", `
			INSERT INTO attachments(target_type, target_id, uploader_id, file_path, mime_type, size_bytes)
			VALUES($1,$2,$3,$4,$5,$6)`,
			func(f *fixture) []any {
				return []any{"comment", f.commentID, f.userID, "comments/1/x.png", "image/png", 123}
			}),
		query("attachment count per target", `SELECT count(*) FROM attachments WHERE target_type=$1 AND target_id=$2`,
			func(f *fixture) []any { return []any{"comment", f.commentID} }),
		query("attachment serve lookup", `SELECT file_path, mime_type, target_type, target_id FROM attachments WHERE id=$1`,
			func(f *fixture) []any { return []any{int64(0)} }),
		query("comment -> list resolution", `
			SELECT t.list_id FROM comments c
			JOIN tasks t ON t.id = c.task_id
			WHERE c.id=$1 AND c.deleted_at IS NULL AND t.archived_at IS NULL`,
			func(f *fixture) []any { return []any{f.commentID} }),

		// --- templates with audience ---
		exec("template insert with audience", `
			INSERT INTO templates(name, body, auto_apply, audience) VALUES($1,$2,$3,$4)`,
			func(f *fixture) []any {
				return []any{"__sqltest tpl", `{"list_name":"x","tasks":[]}`, false, `{"mode":"roles","roles":["user"]}`}
			}),
		query("templates read with audience", `SELECT id, name, body, auto_apply, audience FROM templates ORDER BY id`,
			func(f *fixture) []any { return nil }),

		// --- focus / presence ---
		exec("focus session start", `INSERT INTO focus_sessions(user_id, task_id) VALUES($1,$2)`,
			func(f *fixture) []any { return []any{f.userID, f.taskID} }),
		query("focus current session", `
			SELECT fs.id, fs.task_id, t.title, fs.started_at
			FROM focus_sessions fs
			LEFT JOIN tasks t ON t.id = fs.task_id
			WHERE fs.user_id = $1 AND fs.ended_at IS NULL
			ORDER BY fs.started_at DESC LIMIT 1`,
			func(f *fixture) []any { return []any{f.userID} }),
		// Completing or archiving a task must end its focus session, or the sidebar timer keeps
		// ticking against finished work (a real reported bug).
		exec("close focus on task completion", `
			UPDATE focus_sessions
			SET ended_at = now(), duration_seconds = EXTRACT(EPOCH FROM (now() - started_at))::int
			WHERE task_id = $1 AND ended_at IS NULL
			RETURNING user_id`,
			func(f *fixture) []any { return []any{f.taskID} }),
		exec("close focus on archive (task tree)", `
			UPDATE focus_sessions
			SET ended_at = now(), duration_seconds = EXTRACT(EPOCH FROM (now() - started_at))::int
			WHERE ended_at IS NULL
			  AND task_id IN (SELECT id FROM tasks WHERE id = $1 OR parent_id = $1)
			RETURNING task_id`,
			func(f *fixture) []any { return []any{f.taskID} }),
		exec("focus session stop", `
			UPDATE focus_sessions SET ended_at=now(), duration_seconds=EXTRACT(EPOCH FROM (now()-started_at))::int
			WHERE user_id=$1 AND ended_at IS NULL`,
			func(f *fixture) []any { return []any{f.userID} }),

		// --- profile (post-0008: no theme_scheme) ---
		query("profile read", `
			SELECT display_name, locale, theme_color, theme_visual, avatar_path, notify_prefs
			FROM users WHERE id=$1`,
			func(f *fixture) []any { return []any{f.userID} }),
		exec("profile update", `
			UPDATE users SET
				display_name = COALESCE($2, display_name),
				locale       = COALESCE($3, locale),
				theme_color  = COALESCE($4, theme_color),
				theme_visual = COALESCE($5, theme_visual),
				notify_prefs = CASE WHEN $6::text IS NULL THEN notify_prefs ELSE notify_prefs || $6::jsonb END
			WHERE id=$1`,
			func(f *fixture) []any { return []any{f.userID, "Test", "ru-RU", "blue", "rich", `{"sound":true}`} }),

		// --- search, activity, archive ---
		query("global search (tasks)", `
			SELECT t.id, t.list_id, t.title FROM tasks t
			JOIN lists l ON l.id = t.list_id
			WHERE t.title ILIKE $1 AND t.archived_at IS NULL LIMIT 10`,
			func(f *fixture) []any { return []any{"%sqltest%"} }),
		query("space activity feed", `
			SELECT t.id, t.title, u.username, t.created_at FROM tasks t
			JOIN lists l ON l.id = t.list_id
			JOIN users u ON u.id = t.creator_id
			WHERE l.space_id=$1 AND t.archived_at IS NULL
			ORDER BY t.created_at DESC LIMIT 20`,
			func(f *fixture) []any { return []any{f.spaceID} }),
		exec("archive a task", `UPDATE tasks SET archived_at=now(), archived_by=$2 WHERE id=$1`,
			func(f *fixture) []any { return []any{f.taskID, f.userID} }),
		exec("restore a task", `UPDATE tasks SET archived_at=NULL, archived_by=NULL WHERE id=$1`,
			func(f *fixture) []any { return []any{f.taskID} }),

		// --- watchers / review / replies / rate limits (0009) ---
		exec("watch a task (idempotent)", `
			INSERT INTO task_watchers(task_id, user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`,
			func(f *fixture) []any { return []any{f.taskID, f.userID} }),
		query("list watchers", `
			SELECT u.id, u.username, COALESCE(u.display_name, u.username)
			FROM task_watchers tw JOIN users u ON u.id = tw.user_id
			WHERE tw.task_id = $1 AND u.archived_at IS NULL
			ORDER BY tw.created_at`,
			func(f *fixture) []any { return []any{f.taskID} }),
		query("watcher fan-out (skips actor + assignee)", `
			SELECT tw.user_id FROM task_watchers tw
			JOIN tasks t ON t.id = tw.task_id
			WHERE tw.task_id = $1 AND tw.user_id <> $2
			  AND COALESCE(t.assignee_id, 0) <> tw.user_id`,
			func(f *fixture) []any { return []any{f.taskID, f.userID} }),
		exec("submit for review", `
			UPDATE tasks SET status='review', review_state='pending', review_by=NULL,
				review_at=now(), review_note=NULL, updated_at=now()
			WHERE id=$1`,
			func(f *fixture) []any { return []any{f.taskID} }),
		exec("decide review (accept/return)", `
			UPDATE tasks SET
				review_state=$2, review_by=$3, review_at=now(), review_note=NULLIF($4,''),
				status=$5,
				completed_at = CASE WHEN $2='accepted' THEN COALESCE(completed_at, now()) ELSE NULL END,
				updated_at=now()
			WHERE id=$1`,
			func(f *fixture) []any { return []any{f.taskID, "returned", f.userID, "needs work", "in_progress"} }),
		exec("threaded comment reply", `
			INSERT INTO comments(task_id, author_id, body, parent_id) VALUES($1,$2,$3,$4)`,
			func(f *fixture) []any { return []any{f.taskID, f.userID, "reply", f.commentID} }),
		query("comments with reply threading", `
			SELECT c.id, c.parent_id, c.body FROM comments c
			WHERE c.task_id=$1 AND c.deleted_at IS NULL
			ORDER BY COALESCE(c.parent_id, c.id), c.id`,
			func(f *fixture) []any { return []any{f.taskID} }),
		exec("action counter increment", `
			INSERT INTO action_counters(user_id, action, bucket_hour, count)
			VALUES($1,$2,date_trunc('hour', now()),1)
			ON CONFLICT (user_id, action, bucket_hour) DO UPDATE SET count = action_counters.count + 1`,
			func(f *fixture) []any { return []any{f.userID, "task_create"} }),
		query("action counter read", `
			SELECT COALESCE(sum(count),0)::int FROM action_counters
			WHERE user_id=$1 AND action=$2 AND bucket_hour > now() - interval '1 hour'`,
			func(f *fixture) []any { return []any{f.userID, "task_create"} }),
		exec("action counter prune", `DELETE FROM action_counters WHERE bucket_hour < now() - interval '48 hours'`,
			func(f *fixture) []any { return nil }),

		// --- export / import + integrity (this round) ---
		query("export: tasks with assignee username", `
			SELECT t.id, t.parent_id, t.title, t.description, t.status, COALESCE(t.priority,'normal'),
				u.username, t.start_at, t.due_at, t.weight, t.progress, t.custom_fields, t.completed_at
			FROM tasks t
			LEFT JOIN users u ON u.id = t.assignee_id
			WHERE t.list_id=$1 AND t.archived_at IS NULL
			ORDER BY t.position, t.id`,
			func(f *fixture) []any { return []any{f.listID} }),
		query("export: comments with author", `
			SELECT u.username, c.body, c.is_system, c.created_at
			FROM comments c JOIN users u ON u.id = c.author_id
			WHERE c.task_id=$1 AND c.deleted_at IS NULL ORDER BY c.created_at`,
			func(f *fixture) []any { return []any{f.taskID} }),
		exec("import: task with remapped parent", `
			INSERT INTO tasks(list_id, parent_id, title, description, status, priority,
				assignee_id, start_at, due_at, weight, progress, custom_fields, completed_at, creator_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13,$14)`,
			func(f *fixture) []any {
				return []any{f.listID, nil, "imported", "", "open", "normal",
					nil, nil, nil, 1, nil, "{}", nil, f.userID}
			}),
		query("integrity: start after deadline", `
			SELECT count(*) FROM tasks WHERE start_at IS NOT NULL AND due_at IS NOT NULL AND start_at > due_at`,
			func(f *fixture) []any { return nil }),
		query("integrity: orphaned list members", `
			SELECT count(*) FROM list_members lm LEFT JOIN lists l ON l.id = lm.list_id WHERE l.id IS NULL`,
			func(f *fixture) []any { return nil }),
		query("integrity: orphaned subtasks", `
			SELECT count(*) FROM tasks t WHERE t.parent_id IS NOT NULL
			  AND NOT EXISTS (SELECT 1 FROM tasks p WHERE p.id = t.parent_id)`,
			func(f *fixture) []any { return nil }),
		query("integrity: stale blocked_by ids", `
			SELECT count(*) FROM tasks t
			WHERE COALESCE(array_length(t.blocked_by,1),0) > 0
			  AND EXISTS (
			    SELECT 1 FROM unnest(t.blocked_by) AS dep
			    WHERE NOT EXISTS (SELECT 1 FROM tasks x WHERE x.id = dep)
			  )`,
			func(f *fixture) []any { return nil }),
		query("integrity: dependency cycles", `
			SELECT count(*) FROM tasks a JOIN tasks b ON b.id = ANY(a.blocked_by)
			WHERE a.id = ANY(b.blocked_by)`,
			func(f *fixture) []any { return nil }),
		query("integrity: long-open focus sessions", `
			SELECT count(*) FROM focus_sessions WHERE ended_at IS NULL AND started_at < now() - interval '24 hours'`,
			func(f *fixture) []any { return nil }),
		query("captions: all locales populated", `
			SELECT locale, count(*) FROM stat_captions GROUP BY locale ORDER BY locale`,
			func(f *fixture) []any { return nil }),
		query("notification collapse lookup", `
			SELECT id FROM notifications
			WHERE user_id=$1 AND kind=$2 AND read_at IS NULL
			  AND payload->>'task_id' = $3
			  AND created_at > now() - make_interval(secs => $4)
			ORDER BY created_at DESC LIMIT 1`,
			func(f *fixture) []any { return []any{f.userID, "comment", "1", 120} }),

		query("onboarding progress lookup", `
			SELECT l.id,
				(SELECT count(*) FROM tasks t WHERE t.list_id=l.id AND t.archived_at IS NULL),
				(SELECT count(*) FROM tasks t WHERE t.list_id=l.id AND t.archived_at IS NULL AND t.completed_at IS NOT NULL)
			FROM lists l
			JOIN list_members lm ON lm.list_id = l.id AND lm.user_id = $1 AND lm.permission = 'owner'
			WHERE l.name = 'Onboarding quests' AND l.archived_at IS NULL
			ORDER BY l.id LIMIT 1`,
			func(f *fixture) []any { return []any{f.userID} }),

		// --- telegram linking (0011) ---
		exec("telegram: issue link code", `
			UPDATE users SET telegram_link_code=$2, telegram_link_code_at=now() WHERE id=$1`,
			func(f *fixture) []any { return []any{f.userID, "abc123deadbeef00"} }),
		query("telegram: status lookup", `
			SELECT telegram_chat_id IS NOT NULL FROM users WHERE id=$1`,
			func(f *fixture) []any { return []any{f.userID} }),
		query("telegram: notify target (chat+prefs+locale)", `
			SELECT telegram_chat_id, notify_prefs #>> '{telegram}', locale FROM users WHERE id=$1`,
			func(f *fixture) []any { return []any{f.userID} }),
		exec("telegram: process /start match", `
			UPDATE users SET telegram_chat_id=$2, telegram_link_code=NULL, telegram_link_code_at=NULL
			WHERE telegram_link_code=$1
			  AND telegram_link_code_at > now() - interval '15 minutes'`,
			func(f *fixture) []any { return []any{"abc123deadbeef00", int64(987654321)} }),
		exec("telegram: unlink", `
			UPDATE users SET telegram_chat_id=NULL, telegram_link_code=NULL, telegram_link_code_at=NULL WHERE id=$1`,
			func(f *fixture) []any { return []any{f.userID} }),

		// --- settings ---
		exec("system setting upsert", `
			INSERT INTO system_settings(key,value,updated_at) VALUES($1,$2::jsonb,now())
			ON CONFLICT (key) DO UPDATE SET value=$2::jsonb, updated_at=now()`,
			func(f *fixture) []any { return []any{"__sqltest.key", `"v"`} }),
	}
}

// taskSelectSQL mirrors tasks.go's taskSelect. Kept as a copy on purpose: if the two ever drift,
// this check stops reflecting what the app runs — so it's compared in a test below rather than
// imported across the package boundary.
const taskSelectSQL = `
	SELECT t.id, t.list_id, t.parent_id, t.title, t.description, t.status, t.priority,
		t.assignee_id, t.creator_id, t.start_at, t.due_at, t.weight, t.progress,
		COALESCE(t.blocked_by, '{}'), t.custom_fields, t.recurrence,
		t.completed_at, t.created_at, t.updated_at,
		(SELECT count(*) FROM tasks s WHERE s.parent_id=t.id AND s.archived_at IS NULL AND s.completed_at IS NOT NULL)::int,
		(SELECT count(*) FROM tasks s WHERE s.parent_id=t.id AND s.archived_at IS NULL)::int,
		COALESCE((SELECT json_agg(json_build_object(
				'user_id', u.id, 'username', u.username, 'avatar_path', u.avatar_path, 'started_at', fs.started_at
			) ORDER BY fs.started_at)
			FROM focus_sessions fs JOIN users u ON u.id = fs.user_id
			WHERE fs.task_id = t.id AND fs.ended_at IS NULL), '[]'),
		t.review_state, rv.username, t.review_at, t.review_note,
		(SELECT count(*) FROM task_watchers tw WHERE tw.task_id = t.id)::int
	FROM tasks t
	LEFT JOIN users rv ON rv.id = t.review_by`
