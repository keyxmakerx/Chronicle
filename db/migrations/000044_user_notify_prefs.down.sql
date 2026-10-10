-- Reverse 000044: drop the person's notification choices.
ALTER TABLE users DROP COLUMN IF EXISTS notify_prefs;
