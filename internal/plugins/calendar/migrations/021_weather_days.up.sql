-- 021_weather_days — one weather reading per calendar day.
--
-- calendar_weather (020) keeps only the calendar's current reading; this table
-- holds a reading for any day, past or future. Who may see which day is a
-- service rule (players: up to today), not a schema one.
--
-- source: 'manual' for a reading the Director painted, 'generated' for one the
-- generator wrote. A generated write never replaces a manual reading.
--
-- Not named calendar_day_weather: 008 used that name for a different shape,
-- and IF NOT EXISTS would quietly keep a leftover copy of it.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS calendar_weather_days (
    calendar_id             VARCHAR(36)  NOT NULL,
    year                    INT          NOT NULL,
    month                   INT          NOT NULL,
    day                     INT          NOT NULL,
    preset_id               VARCHAR(50)  DEFAULT NULL,
    preset_label            VARCHAR(100) DEFAULT NULL,
    icon                    VARCHAR(50)  DEFAULT NULL,
    color                   VARCHAR(20)  DEFAULT NULL,
    temperature_celsius     FLOAT        DEFAULT NULL,
    wind_speed_kph          FLOAT        DEFAULT NULL,
    wind_speed_tier         VARCHAR(20)  DEFAULT NULL,
    wind_direction          VARCHAR(5)   DEFAULT NULL,
    wind_direction_degrees  INT          DEFAULT NULL,
    precipitation_type      VARCHAR(20)  DEFAULT NULL,
    precipitation_intensity FLOAT        DEFAULT NULL,
    zone_id                 VARCHAR(50)  DEFAULT NULL,
    zone_name               VARCHAR(100) DEFAULT NULL,
    description             TEXT         DEFAULT NULL,
    source                  VARCHAR(10)  NOT NULL DEFAULT 'manual',
    updated_at              DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (calendar_id, year, month, day),
    CONSTRAINT fk_cal_weather_days_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
