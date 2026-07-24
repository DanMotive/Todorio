-- Track who archived each task/list/space, so the Archive view can show it and
-- the background worker can warn that specific person 3 days before permanent
-- auto-cleanup (see internal/worker/worker.go: warnPendingCleanup). Items
-- archived before this migration simply have archived_by=NULL and are still
-- cleaned up on schedule — they just don't get a targeted warning, since we
-- don't know who to send it to.
ALTER TABLE tasks  ADD COLUMN IF NOT EXISTS archived_by BIGINT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE lists  ADD COLUMN IF NOT EXISTS archived_by BIGINT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE spaces ADD COLUMN IF NOT EXISTS archived_by BIGINT REFERENCES users(id) ON DELETE SET NULL;
