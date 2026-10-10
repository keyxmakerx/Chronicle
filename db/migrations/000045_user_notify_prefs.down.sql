-- Reverse 000045: drop the person's notification choices.
ALTER TABLE users DROP COLUMN IF EXISTS notify_prefs;
