-- C-CAL-WEATHER-ZONES (Wave 0 PR 3): per-calendar weather zone
-- definitions. Each zone is a named climate region (e.g. "temperate",
-- "tropical", "arctic") with a JSON payload holding presets +
-- season-overrides + any future fields.
--
-- Scope decision: zones are CALENDAR-scoped, not campaign-scoped.
-- The dispatch's original `campaign_id` framing predated V2's multi-
-- calendar architecture (one campaign → N calendars; each calendar
-- has its own weather state). The existing `calendar_weather` table
-- is calendar-scoped (UNIQUE on calendar_id at
-- migrations/003_weather_cycles_festivals.up.sql); zones follow the
-- same scoping so the active-zone reference (calendar_weather.zone_id
-- already present from migration 003) keys cleanly into this table
-- via (calendar_id, zone_id).
--
-- Status-report cites: stop-and-flag #1 ("existing calendar_weather
-- doesn't have campaign_id") confirmed — calendar_weather is
-- calendar-scoped; zones follow.
--
-- Per cordinator/dispatches/chronicle/C-CAL-WEATHER-ZONES.md §"Storage
-- shape" (translated from PostgreSQL JSONB / TIMESTAMPTZ to MariaDB
-- JSON / DATETIME).

CREATE TABLE IF NOT EXISTS calendar_weather_zones (
    calendar_id  VARCHAR(36)  NOT NULL,
    zone_id      VARCHAR(50)  NOT NULL,
    name         VARCHAR(100) NOT NULL,
    payload      JSON         NOT NULL,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (calendar_id, zone_id),
    INDEX idx_calendar_weather_zones_calendar (calendar_id),
    CONSTRAINT fk_calendar_weather_zones_calendar
      FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
