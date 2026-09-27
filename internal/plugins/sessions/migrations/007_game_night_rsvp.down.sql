-- Revert 007_game_night_rsvp. Tables first, in reverse dependency order, then
-- the columns added to the pre-existing tables.
--
-- CAUTION: dropping `deleted_at` un-deletes every session that was soft
-- deleted while this migration was applied — they have no other record of
-- having been removed, so they come back as live, ordinary sessions. Confirm
-- there are no soft-deleted rows (or that reviving them is acceptable) before
-- rolling this back on a database that has taken live traffic.
DROP TABLE IF EXISTS session_calendar_feed_settings;
DROP TABLE IF EXISTS session_calendar_feed_tokens;
DROP TABLE IF EXISTS session_reschedule_suggestions;
DROP TABLE IF EXISTS session_occurrence_rsvps;

ALTER TABLE session_attendees
  DROP COLUMN IF EXISTS needs_recheck,
  DROP COLUMN IF EXISTS excluded_from_count,
  DROP COLUMN IF EXISTS note;

ALTER TABLE sessions DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE sessions DROP COLUMN IF EXISTS scheduled_tz;
