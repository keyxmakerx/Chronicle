-- C-SYNC-APPLIED-BEACON: upgrades the served-date beacon (#548,
-- 005_calendar_date_beacon) from "saw" to "applied". #548 proved what
-- Foundry last SAW (a Bearer-authed GET /calendar/date read); it never
-- proved the module actually applied that date to its own calendar state
-- (booked as follow-up FM-SYNC-CONFIRMED-DATE). POST /calendar/date/confirm
-- (calendar_api_handler.go ConfirmDate) writes these columns after the
-- module has actually set its date.
--
-- Onto the SAME per-campaign row, not a new table — a campaign either saw
-- or applied a date, and "applied" is meaningless without the row the
-- serving side already keyed by campaign_id. All four columns are NULL
-- until the first confirm, independent of whether last_served_* has ever
-- been populated: a confirm may arrive before any GET has landed for this
-- campaign on a fresh install, and repository.go's
-- ConfirmCalendarDateBeacon create-or-updates this row without touching
-- last_served_* on the update path (see its doc comment for why the
-- create path fills last_served_* with the 0/0 "unset" sentinel already
-- used elsewhere in this codebase, e.g. calendar_api_handler.go's
-- defaultIfZero, rather than faking a served date).

ALTER TABLE sync_calendar_date_beacons
    ADD COLUMN IF NOT EXISTS applied_year  INT      NULL,
    ADD COLUMN IF NOT EXISTS applied_month INT      NULL,
    ADD COLUMN IF NOT EXISTS applied_day   INT      NULL,
    ADD COLUMN IF NOT EXISTS applied_at    DATETIME NULL;
