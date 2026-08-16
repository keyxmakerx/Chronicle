-- Reverse C-CALV4-ANCHOR: drop the real-date anchor columns. IF EXISTS keeps
-- the rollback idempotent (safe to re-run / partial-apply recovery).
ALTER TABLE calendars
  DROP COLUMN IF EXISTS anchor_real_date,
  DROP COLUMN IF EXISTS anchor_day,
  DROP COLUMN IF EXISTS anchor_month,
  DROP COLUMN IF EXISTS anchor_year;
