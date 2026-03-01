-- Rollback Sprint 6: Remove mode and timezone columns.

ALTER TABLE users
  DROP COLUMN timezone;

ALTER TABLE calendars
  DROP COLUMN mode;
