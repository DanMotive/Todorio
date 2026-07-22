-- Todorio · 0004_productivity · notes, favorites, saved filters, focus sessions, share-link passwords.

-- Markdown notes pages, scoped to a space (optionally to one list within it).
CREATE TABLE notes (
    id          BIGSERIAL PRIMARY KEY,
    space_id    BIGINT NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    list_id     BIGINT REFERENCES lists(id) ON DELETE CASCADE,
    title       TEXT NOT NULL DEFAULT 'Untitled',
    body        TEXT NOT NULL DEFAULT '',
    created_by  BIGINT NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at TIMESTAMPTZ
);
CREATE INDEX idx_notes_space ON notes(space_id) WHERE archived_at IS NULL;
CREATE INDEX idx_notes_list ON notes(list_id) WHERE archived_at IS NULL;

-- Favorites / pinned items: tasks or lists starred by a user (shown first, quick access).
CREATE TABLE favorites (
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_type TEXT NOT NULL CHECK (target_type IN ('task','list')),
    target_id   BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, target_type, target_id)
);

-- Saved filters: reusable query definitions per user, for one list or globally ("My tasks").
CREATE TABLE saved_filters (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    list_id     BIGINT REFERENCES lists(id) ON DELETE CASCADE, -- NULL = global filter
    name        TEXT NOT NULL,
    query       JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_saved_filters_user ON saved_filters(user_id);

-- Focus mode sessions: simple time tracking, optionally tied to one task.
CREATE TABLE focus_sessions (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    task_id          BIGINT REFERENCES tasks(id) ON DELETE SET NULL,
    started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at         TIMESTAMPTZ,
    duration_seconds INT
);
CREATE INDEX idx_focus_sessions_user ON focus_sessions(user_id);

-- Optional password for public read-only share links (/s/{token}).
ALTER TABLE share_links ADD COLUMN password_hash TEXT;
