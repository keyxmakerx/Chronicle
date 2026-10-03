-- 023_weather_settings — one weather setting row per calendar.
--
-- climate is the world's climate (an id from WeatherClimates, validated by the
-- service, not the schema, so a new climate needs no migration). continuity is
-- how long weather lasts: 0 changes every day, 1 settles into long spells.
-- No row means the defaults (temperate, 0.55).
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS calendar_weather_settings (
    calendar_id VARCHAR(36)  NOT NULL PRIMARY KEY,
    climate     VARCHAR(40)  NOT NULL,
    continuity  DECIMAL(3,2) NOT NULL DEFAULT 0.55,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    CONSTRAINT fk_cal_weather_settings_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
