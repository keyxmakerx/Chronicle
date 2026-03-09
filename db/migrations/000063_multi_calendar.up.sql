-- Multi-Calendar Support
-- Allows unlimited calendars per campaign (was limited to exactly one).
-- Adds sort_order and is_default columns to calendars table.
-- Adds calendar_id FK to sessions for explicit calendar association.

-- 1. Drop the UNIQUE constraint that limited to one calendar per campaign.
ALTER TABLE calendars DROP INDEX idx_calendars_campaign;

-- 2. Add a regular (non-unique) index for campaign lookups.
ALTER TABLE calendars ADD INDEX idx_calendars_campaign (campaign_id);

-- 3. Add ordering and default flag columns.
ALTER TABLE calendars ADD COLUMN sort_order INT NOT NULL DEFAULT 0 AFTER epoch_name;
ALTER TABLE calendars ADD COLUMN is_default TINYINT(1) NOT NULL DEFAULT 0 AFTER sort_order;

-- 4. Mark all existing calendars as the default for their campaign.
UPDATE calendars SET is_default = 1;

-- 5. Add calendar_id FK to sessions for explicit calendar association.
ALTER TABLE sessions ADD COLUMN calendar_id VARCHAR(36) DEFAULT NULL AFTER campaign_id;
ALTER TABLE sessions ADD CONSTRAINT fk_sessions_calendar
    FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE SET NULL;

-- 6. Backfill: link existing sessions to their campaign's single calendar.
UPDATE sessions s
    JOIN calendars c ON c.campaign_id = s.campaign_id
    SET s.calendar_id = c.id
    WHERE s.calendar_year IS NOT NULL;
