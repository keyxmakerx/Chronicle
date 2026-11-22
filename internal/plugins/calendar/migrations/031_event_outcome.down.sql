-- 031_event_outcome (down) — drop what became of each event and the
-- calendar's switch. Every event then reads as planned, as before. The
-- lines on what happened are lost with the column; take a backup first if
-- they matter. Idempotent.
ALTER TABLE calendars
  DROP COLUMN IF EXISTS ask_when_day_passes;

ALTER TABLE calendar_events
  DROP COLUMN IF EXISTS moved_from_day,
  DROP COLUMN IF EXISTS moved_from_month,
  DROP COLUMN IF EXISTS moved_from_year,
  DROP COLUMN IF EXISTS outcome,
  DROP COLUMN IF EXISTS status;
