-- Reverse C-REAL-CALENDAR-P1: drop the real-time columns. IF EXISTS keeps the
-- rollback idempotent (safe to re-run / partial-apply recovery).
ALTER TABLE calendars
  DROP COLUMN IF EXISTS tracks_real_time,
  DROP COLUMN IF EXISTS real_time_zone;
