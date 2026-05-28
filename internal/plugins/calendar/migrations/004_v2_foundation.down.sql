-- Reverse C-CAL-V2-SCHEMA-FOUNDATION: drop tier column + drop is_default
-- partial-unique infrastructure (virtual generated column + its UNIQUE index).

ALTER TABLE calendars
  DROP INDEX idx_one_default_per_campaign,
  DROP COLUMN default_marker;

ALTER TABLE calendar_events
  DROP INDEX idx_calendar_events_tier,
  DROP COLUMN tier;
