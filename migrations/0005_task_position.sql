-- Todorio · 0005_task_position · fixes a real bug found in testing.
--
-- internal/api/tasks.go has always ordered lists by "ORDER BY t.position, t.id" and updated a
-- "position" field on PATCH — but no migration ever created tasks.position (only lists.position
-- exists, from 0001). Every call to GET /api/lists/{id}/tasks and every PATCH /api/tasks/{id}
-- was failing with a Postgres "column t.position does not exist" error. Symptom in the UI:
-- creating a task in a list "does nothing" (the INSERT itself succeeds — it never touched
-- position — but the immediate list refresh 500s and is swallowed client-side, so the new task
-- never appears), and separately every task edit/status toggle/etc. was silently failing too.
--
-- This is a new migration rather than editing 0001 because 0001-0004 are likely already applied
-- (tracked in schema_migrations) on any server that has already run `todorio setup` — editing an
-- already-applied migration file has no effect on an existing database. This one is additive and
-- picked up automatically on the next `todorio serve` / service restart.

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS position INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_tasks_list_position ON tasks(list_id, position) WHERE archived_at IS NULL;
