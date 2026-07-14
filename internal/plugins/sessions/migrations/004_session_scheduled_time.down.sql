-- Reverse C-SCHED-P3 Slice 3. IF EXISTS keeps the rollback idempotent (safe to
-- re-run / partial-apply recovery).
ALTER TABLE sessions DROP COLUMN IF EXISTS scheduled_time;
