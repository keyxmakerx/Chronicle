-- A person's notification choices (which kinds of message reach them on the
-- bell and by email): a JSON object, NULL until they change one. On the user
-- row so the choices follow the person; unknown stored values are ignored on
-- read. Safety mail is not governed by it.
-- Core table, core migration: no plugin tables referenced.

ALTER TABLE users ADD COLUMN IF NOT EXISTS notify_prefs JSON NULL;
