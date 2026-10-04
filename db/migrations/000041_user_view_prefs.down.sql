-- Reverse 000038: drop the person's own viewing choices.
ALTER TABLE users DROP COLUMN IF EXISTS view_prefs;
