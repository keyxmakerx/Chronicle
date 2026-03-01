-- Revert: remove time system columns from calendars and events.

ALTER TABLE calendar_events
  DROP COLUMN end_minute,
  DROP COLUMN end_hour,
  DROP COLUMN start_minute,
  DROP COLUMN start_hour;

ALTER TABLE calendars
  DROP COLUMN current_minute,
  DROP COLUMN current_hour,
  DROP COLUMN seconds_per_minute,
  DROP COLUMN minutes_per_hour,
  DROP COLUMN hours_per_day;
