-- Reverse C-SYNC-APPLIED-BEACON: drop the applied-date columns. Safe to
-- roll back for the same reason as 005's down migration — the beacon is a
-- derived diagnostic signal, re-populated by the next confirm. Losing it
-- only reverts the sync chip to preferring the served-date beacon, matching
-- pre-C-SYNC-APPLIED-BEACON behavior.

ALTER TABLE sync_calendar_date_beacons
    DROP COLUMN IF EXISTS applied_at,
    DROP COLUMN IF EXISTS applied_day,
    DROP COLUMN IF EXISTS applied_month,
    DROP COLUMN IF EXISTS applied_year;
