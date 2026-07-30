-- 0018: data-integrity guards for the registration races fixed in the API layer.
--
-- 1. There must never be more than one root. The old bootstrap ("first user becomes root")
--    checked count(*) and inserted in separate statements, so two concurrent first
--    registrations could both win. If a database was already affected, keep the OLDEST root
--    and demote the later ones to admin — demotion is recoverable by the remaining root,
--    silently deleting an account would not be.
UPDATE users
SET role = 'admin'
WHERE role = 'root'
  AND id NOT IN (SELECT min(id) FROM users WHERE role = 'root');

CREATE UNIQUE INDEX IF NOT EXISTS users_single_root_key
    ON users ((role)) WHERE role = 'root';

-- 2. An invite must not be redeemable more times than max_uses. lookup + consume used to be
--    two separate statements, so a burst of concurrent registrations could overshoot. Clamp
--    any overshoot that already happened, then make it impossible at the row level.
UPDATE invites SET used_count = max_uses WHERE used_count > max_uses;

ALTER TABLE invites
    ADD CONSTRAINT invites_used_count_bounds CHECK (used_count >= 0 AND used_count <= max_uses);
