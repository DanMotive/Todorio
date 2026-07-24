-- The light colour scheme was removed from the product: Todorio is dark-only now, so
-- users.theme_scheme (added in 0001 with a CHECK on 'light'/'dark') has no reader left.
--
-- This drop is deliberate and irreversible — it was confirmed rather than assumed, since the
-- column holds real per-user preferences on the live database. Nothing else references it:
-- the API stopped selecting and updating it in the same change, and no view or index depends
-- on it. IF EXISTS keeps the migration idempotent on a database where it was already applied
-- or on a fresh install that never had the column.
ALTER TABLE users DROP COLUMN IF EXISTS theme_scheme;
