-- Functional additions: personal Telegram bots, note-to-task links.
--
-- Unblock notifications and the workload view need no schema at all: blocked_by, weight and
-- due_at already exist, they were simply never read for these purposes.

-- Personal Telegram bot per user.
--
-- Until now one bot token was configured by root for the whole instance (system_settings
-- 'telegram.bot_token'), so a user could only receive notifications if the administrator had
-- set one up. These columns let anyone paste a token from @BotFather and get their own
-- delivery. The instance-wide bot keeps working exactly as before and is used whenever a user
-- has not configured a personal one.
--
-- The token is a credential: it is never returned to the browser (only the bot's @username and
-- a masked tail), never written to the audit log, and never leaves the server except in
-- requests to api.telegram.org. It is stored in plaintext, exactly like the instance-wide token
-- in system_settings — the project has no key-management story, and inventing one that keeps
-- the key next to the ciphertext on the same disk would be theatre rather than protection.
-- Anyone with database access can already read every session and password hash.
ALTER TABLE users ADD COLUMN IF NOT EXISTS telegram_bot_token TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS telegram_bot_username TEXT NOT NULL DEFAULT '';

-- Where a task came from, when it was created out of a note.
--
-- ON DELETE SET NULL rather than CASCADE: deleting a note must never delete real work that was
-- extracted from it. The task simply stops pointing anywhere.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS source_note_id BIGINT REFERENCES notes(id) ON DELETE SET NULL;

-- Partial index: the column is NULL for almost every task, and the only query is "which tasks
-- came from this note".
CREATE INDEX IF NOT EXISTS tasks_source_note_idx ON tasks(source_note_id)
	WHERE source_note_id IS NOT NULL;
