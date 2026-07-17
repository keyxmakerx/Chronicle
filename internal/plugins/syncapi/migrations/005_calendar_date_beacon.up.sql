-- Served-date beacon for the Calendaria sync chip's dormant DRIFT state
-- (C-SYNC-DATE-BEACON, closes chronicle#545's flagged gap).
--
-- Records the date Foundry last SAW when GET /api/v1/campaigns/:id/calendar/date
-- (calendar_api_handler.go GetCurrentDate, the endpoint the Foundry module
-- polls) is read by a real Bearer-authed API key. Naming is deliberately
-- "served", not "applied" or "synced" — this table proves what the module
-- last read, never what it did with it.
--
-- Grain is per-campaign (one row per campaign_id), so a new table was
-- needed: api_keys is per-key (a campaign can have several), and
-- sync_mappings is per-synced-object — neither fits a single per-campaign
-- "last thing this campaign served" value. This is the wave's one
-- authorized migration (C-SYNC-DATE-BEACON dispatch).

CREATE TABLE IF NOT EXISTS sync_calendar_date_beacons (
    campaign_id        CHAR(36)  NOT NULL PRIMARY KEY,
    last_served_year   INT       NOT NULL,
    last_served_month  INT       NOT NULL,
    last_served_day    INT       NOT NULL,
    last_served_at     DATETIME  NOT NULL,

    CONSTRAINT fk_sync_calendar_date_beacons_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
