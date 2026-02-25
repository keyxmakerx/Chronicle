-- Reverse calendar v2 + device fingerprint changes.

ALTER TABLE api_keys
    DROP COLUMN device_bound_at,
    DROP COLUMN device_fingerprint;

ALTER TABLE calendar_events
    DROP COLUMN category,
    DROP COLUMN end_day,
    DROP COLUMN end_month,
    DROP COLUMN end_year;

ALTER TABLE calendar_seasons
    DROP COLUMN color;

ALTER TABLE calendar_months
    DROP COLUMN leap_year_days;

ALTER TABLE calendars
    DROP COLUMN leap_year_offset,
    DROP COLUMN leap_year_every;
