-- Custom wallpaper per user (see internal/api/wallpaper.go).
--
-- One picture per user, so a column on users rather than a table, exactly like avatar_path.
-- NULL means "no uploaded wallpaper": the built-in gradients and the system wallpapers are
-- picked in the browser and need no server state at all.

ALTER TABLE users ADD COLUMN IF NOT EXISTS wallpaper_path TEXT;
