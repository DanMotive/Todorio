-- Timeline/Gantt view (spec section 12) needs a start date: a bar is drawn from a task's
-- start to its deadline, and until now a task only had due_at.
--
-- Nullable on purpose. A task with no start date is still perfectly valid — most tasks are
-- just "due by X" — and the Timeline falls back to a sensible implied start for those
-- (see internal/api/timeline.go) rather than hiding them. Backfilling every existing task
-- with a guessed start would invent schedule data the user never entered.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS start_at TIMESTAMPTZ;

-- The Timeline queries a date window per list/space, so it filters on the two date columns
-- together. This index serves that range scan; due_at alone had no index either.
CREATE INDEX IF NOT EXISTS tasks_schedule_idx ON tasks (list_id, start_at, due_at)
    WHERE archived_at IS NULL;
