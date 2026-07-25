-- 0012: security hardening (follow-up to the auth audit).
--
-- Everything here is additive: new nullable columns, new defaults, one new table and two
-- indexes. No existing column changes type or meaning, so an older binary keeps working
-- against a database that has already been migrated.

-- --- TOTP -------------------------------------------------------------------------------

-- A secret that has been generated but not yet confirmed with a code. Setup writes here and
-- never touches totp_secret/totp_enabled, so starting a new enrolment can no longer switch off
-- the second factor that is already active.
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_pending_secret TEXT;

-- Highest 30-second counter already accepted for this user; anything at or below it is a
-- replay and is refused even though the code is still inside the ±1 window.
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_last_counter BIGINT;

-- Throttling for code entry. The login endpoint has its own in-memory limiter, but the
-- enable/disable endpoints are reached with a valid session and need their own counter.
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_fail_count INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_locked_until TIMESTAMPTZ;

-- Single-use recovery codes, so losing the authenticator app is not an account loss.
-- Only the hash is stored; the plaintext is shown once, at enable time.
CREATE TABLE IF NOT EXISTS totp_recovery_codes (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash  TEXT NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS totp_recovery_codes_unused_idx
    ON totp_recovery_codes(user_id) WHERE used_at IS NULL;

-- --- sessions ---------------------------------------------------------------------------

-- Sliding expiry: a session in daily use is extended instead of dropping the user at the
-- 30-day mark, and one that goes quiet still expires on schedule.
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- enforceSessionLimit and the "sign out everywhere" paths both filter by user_id.
CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions(user_id);

-- --- usernames --------------------------------------------------------------------------

-- Usernames are compared case-insensitively from now on, so "Admin" can no longer be
-- registered alongside "admin" and impersonate it in mentions and comment bylines.
--
-- An instance that already contains such a pair cannot have the unique index applied without
-- losing an account, so this skips with a warning rather than failing the upgrade. Resolve the
-- collision by renaming one of the accounts and the index is created on the next start.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM (
            SELECT lower(username) AS u FROM users GROUP BY 1 HAVING count(*) > 1
        ) dupes
    ) THEN
        RAISE WARNING 'Todorio: usernames differing only by case already exist; skipping the case-insensitive unique index. Rename one of the accounts to enable it.';
    ELSE
        CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_key ON users (lower(username));
    END IF;
END $$;
