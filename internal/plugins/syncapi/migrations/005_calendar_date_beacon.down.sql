-- Drops the served-date beacon table. Safe to roll back: the beacon is a
-- derived diagnostic signal (re-populated on the next Bearer-authed
-- GET /calendar/date poll), not source-of-truth campaign data. Losing it
-- only reverts the sync chip's drift detection to dormant, matching
-- pre-C-SYNC-DATE-BEACON behavior.

DROP TABLE IF EXISTS sync_calendar_date_beacons;
