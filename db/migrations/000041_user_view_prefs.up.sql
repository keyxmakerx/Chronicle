-- A person's own viewing choices (light/dark, calmer motion, text size,
-- contrast): a JSON object of the non-default choices, NULL when they have
-- made none. It lives on the user row so the choices follow the person to
-- every device and never touch a campaign's settings or any other member's
-- view. Unknown or invalid stored values are ignored on read.
-- Core table, core migration: no plugin tables referenced.

ALTER TABLE users ADD COLUMN IF NOT EXISTS view_prefs JSON NULL;
