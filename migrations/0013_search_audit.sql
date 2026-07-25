-- 0013: full-text search indexes + an audit log for administrative actions.
--
-- Part 1: search
--
-- Global search ran three `ILIKE '%term%'` queries. A leading wildcard makes a btree index
-- useless, so every search was a sequential scan over tasks, notes and comments — fine with a
-- few hundred rows, visibly slow once a workspace has real history.
--
-- The text is now mirrored into a stored tsvector column with a GIN index. Two deliberate
-- choices here:
--
--   * The 'simple' text search configuration, not 'english'/'russian'. Todorio is used in 13
--     languages and a row's language is not known at write time; a stemmer for the wrong
--     language is worse than no stemmer. 'simple' just folds case and splits on word
--     boundaries. The lost stemming is compensated in the query: search.go appends `:*` to
--     every term, so "задач" still matches "задачи" and "deploy" matches "deployment".
--   * Generated columns rather than triggers, so the vector can never drift out of sync with
--     the text it indexes. This requires PostgreSQL 12 or newer.
--
-- ADD COLUMN ... GENERATED ... STORED rewrites the table. On a self-hosted instance of this
-- size that is a one-off pause of seconds, not minutes, and it happens during the normal
-- migration step at startup.
--
-- Search behaviour is unchanged for the caller: search.go falls back to the old ILIKE queries
-- whenever the full-text pass finds nothing, so substring matches inside a word (and CJK text,
-- which 'simple' does not segment) keep working exactly as before.

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS search_tsv tsvector
    GENERATED ALWAYS AS (
        to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(description, ''))
    ) STORED;

ALTER TABLE notes ADD COLUMN IF NOT EXISTS search_tsv tsvector
    GENERATED ALWAYS AS (
        to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(body, ''))
    ) STORED;

ALTER TABLE comments ADD COLUMN IF NOT EXISTS search_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', coalesce(body, ''))) STORED;

CREATE INDEX IF NOT EXISTS tasks_search_idx ON tasks USING GIN (search_tsv);
CREATE INDEX IF NOT EXISTS notes_search_idx ON notes USING GIN (search_tsv);
CREATE INDEX IF NOT EXISTS comments_search_idx ON comments USING GIN (search_tsv);

-- Part 2: admin audit log
--
-- Blocking a user, changing someone's role, resetting a password, permanently deleting a space
-- and flipping a server policy left no trace anywhere. With several admins in a workspace there
-- was no way to answer "who did this, and when".
--
-- actor_username is stored as a snapshot alongside actor_id on purpose: the point of an audit
-- trail is that it survives the account being renamed or deleted, and ON DELETE SET NULL would
-- otherwise erase the only readable trace of who acted.
--
-- details is free-form JSON per action (old/new role, target username, setting key, ...). It
-- must never receive secrets: password resets record that a reset happened, never the password.

CREATE TABLE IF NOT EXISTS admin_audit (
    id             BIGSERIAL PRIMARY KEY,
    actor_id       BIGINT REFERENCES users(id) ON DELETE SET NULL,
    actor_username TEXT NOT NULL DEFAULT '',
    action         TEXT NOT NULL,
    target_type    TEXT NOT NULL DEFAULT '',   -- user | task | list | space | setting | locale
    target_id      BIGINT,
    details        JSONB NOT NULL DEFAULT '{}',
    ip             TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The log is read newest-first, occasionally filtered by actor.
CREATE INDEX IF NOT EXISTS admin_audit_created_idx ON admin_audit (created_at DESC);
CREATE INDEX IF NOT EXISTS admin_audit_actor_idx ON admin_audit (actor_id, created_at DESC);
