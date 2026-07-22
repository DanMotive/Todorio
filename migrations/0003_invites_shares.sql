-- 0003: invites and public read-only links

-- Invites: the code grants instant activation without manual approval.
-- Whether regular users can create invites is controlled by policy.users.can_invite (default false).
CREATE TABLE IF NOT EXISTS invites (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE,
    created_by  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user','viewer','admin')),
    max_uses    INT NOT NULL DEFAULT 1 CHECK (max_uses BETWEEN 1 AND 100),
    used_count  INT NOT NULL DEFAULT 0,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Public read-only links to lists: /s/{token} (no authentication).
-- Enabled/disabled globally via policy.sharing.public_links (default true).
CREATE TABLE IF NOT EXISTS share_links (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token       TEXT NOT NULL UNIQUE,
    list_id     BIGINT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    created_by  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS share_links_list_idx ON share_links(list_id);
