-- Calendar v2: leap year support, event end dates, season colors, event categories.
-- Also: device fingerprint binding for API keys (single-device registration).

-- Leap year support on calendars: every N years, add extra days to specified months.
ALTER TABLE calendars
    ADD COLUMN leap_year_every INT NOT NULL DEFAULT 0 AFTER current_day,
    ADD COLUMN leap_year_offset INT NOT NULL DEFAULT 0 AFTER leap_year_every;

-- Per-month leap year extra days (how many days to add during a leap year).
ALTER TABLE calendar_months
    ADD COLUMN leap_year_days INT NOT NULL DEFAULT 0 AFTER is_intercalary;

-- Season display color for visual indicators on the calendar grid.
ALTER TABLE calendar_seasons
    ADD COLUMN color VARCHAR(7) NOT NULL DEFAULT '#6b7280' AFTER description;

-- Event end dates for multi-day events (e.g. festivals, quests, travel).
ALTER TABLE calendar_events
    ADD COLUMN end_year INT DEFAULT NULL AFTER day,
    ADD COLUMN end_month INT DEFAULT NULL AFTER end_year,
    ADD COLUMN end_day INT DEFAULT NULL AFTER end_month,
    ADD COLUMN category VARCHAR(50) DEFAULT NULL AFTER visibility;

-- Index for reverse entity-event lookup (find all events for an entity).
-- The existing idx_cal_events_entity index suffices for the FK but a covering
-- index helps the reverse lookup query return faster.

-- Device fingerprint binding for API keys: the first device that authenticates
-- with a key gets its fingerprint recorded. Subsequent requests from different
-- devices are rejected. This ensures single-device registration.
ALTER TABLE api_keys
    ADD COLUMN device_fingerprint VARCHAR(255) DEFAULT NULL AFTER ip_allowlist,
    ADD COLUMN device_bound_at DATETIME DEFAULT NULL AFTER device_fingerprint;
