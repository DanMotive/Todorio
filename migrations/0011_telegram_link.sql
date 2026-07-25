-- Telegram notification delivery (root supplies a bot token; each user links their own chat).
-- See internal/telegram for the bot client + linking poll loop, and api.go's notify() for the
-- delivery hook. The bot token and the last processed getUpdates offset live in system_settings
-- (telegram.bot_token, telegram.bot_username, telegram.last_update_id) — no schema for those,
-- they're server-wide singletons like every other root setting.

ALTER TABLE users ADD COLUMN telegram_chat_id BIGINT;
ALTER TABLE users ADD COLUMN telegram_link_code TEXT;
ALTER TABLE users ADD COLUMN telegram_link_code_at TIMESTAMPTZ;

-- A chat can belong to at most one account, and a pending code identifies at most one account —
-- both partial (NULL-excluding) so unlinked users don't collide with each other on NULL.
CREATE UNIQUE INDEX users_telegram_chat_id_idx ON users(telegram_chat_id) WHERE telegram_chat_id IS NOT NULL;
CREATE UNIQUE INDEX users_telegram_link_code_idx ON users(telegram_link_code) WHERE telegram_link_code IS NOT NULL;
