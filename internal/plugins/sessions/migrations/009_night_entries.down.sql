-- Revert 009_night_entries. The Journal entries stay as ordinary notes; only
-- their link to a game night goes.
DROP TABLE IF EXISTS session_night_entries;
