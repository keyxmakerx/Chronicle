-- Reverse C-SCHED-P1 Slice 1. IF EXISTS keeps the rollback idempotent
-- (safe to re-run / partial-apply recovery).
DROP TABLE IF EXISTS availability_exceptions;
DROP TABLE IF EXISTS member_availability;
