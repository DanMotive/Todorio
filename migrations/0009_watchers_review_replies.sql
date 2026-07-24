-- Round: watchers, a real review workflow, threaded comment replies, and action rate limits.

-- Watchers (spec section 5: "наблюдатели" is listed among a task's fields but never existed).
-- A watcher follows a task without owning it: they get the same notifications the assignee does.
-- Composite primary key so "watch" is idempotent — clicking it twice cannot create duplicates.
CREATE TABLE IF NOT EXISTS task_watchers (
    task_id    BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, user_id)
);
-- Notification fan-out reads by task; the "what am I watching" screen reads by user.
CREATE INDEX IF NOT EXISTS task_watchers_user_idx ON task_watchers (user_id);

-- Review mode (spec section 5: "режим «на проверке» (владелец принимает / возвращает)").
-- Until now "review" was only a status string with no accept/return semantics and no record of
-- who decided what. These columns hold that decision.
--
-- review_state is NULL for tasks that were never submitted, so existing rows are untouched:
--   NULL      — not in review
--   pending   — submitted, waiting for a decision
--   accepted  — approved by a reviewer
--   returned  — sent back for rework, with a reason
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS review_state TEXT
    CHECK (review_state IS NULL OR review_state IN ('pending', 'accepted', 'returned'));
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS review_by BIGINT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS review_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS review_note TEXT;

-- Threaded comment replies (spec section 7 lists replies as post-v1; the column is additive and
-- a NULL parent_id means a top-level comment, so every existing comment stays valid).
-- ON DELETE CASCADE: deleting a comment removes the thread hanging off it rather than leaving
-- orphans pointing at a missing parent.
ALTER TABLE comments ADD COLUMN IF NOT EXISTS parent_id BIGINT REFERENCES comments(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS comments_parent_idx ON comments (parent_id) WHERE parent_id IS NOT NULL;

-- Action rate limits (spec section 10: "не более N задач/файлов за период").
-- One row per action per user per hour bucket, incremented on write. Deliberately coarse: an
-- hourly counter is enough to stop runaway automation without the cost of a full audit log.
CREATE TABLE IF NOT EXISTS action_counters (
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action      TEXT   NOT NULL,          -- e.g. 'task_create', 'upload'
    bucket_hour TIMESTAMPTZ NOT NULL,     -- start of the hour this count belongs to
    count       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, action, bucket_hour)
);
-- Old buckets are pruned by the background worker; this index makes that sweep cheap.
CREATE INDEX IF NOT EXISTS action_counters_bucket_idx ON action_counters (bucket_hour);
