-- 032_place_weather — weather for each place that has a climate of its own,
-- and where the party is.
--
-- A place's climate is a field on its page (the entities plugin's climate
-- field), so it needs no column here. What the calendar keeps is the weather
-- that climate gives, day by day, and the days the party arrived somewhere.
--
-- calendar_place_weather_days: one reading per place per day, the same
-- columns and the same rules as calendar_weather_days (021, 025): a
-- generated write never replaces a manual or locked reading. place_id is the
-- page that holds the climate; places inside it read its weather.
-- calendar_weather_days stays the weather everywhere else.
--
-- calendar_party_places: from each marked day on, the party is at place_id,
-- until the next mark. NULL place_id means nowhere with a climate of its
-- own, so the world's weather. Deleting a page empties its marks rather than
-- losing the dates.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS calendar_place_weather_days (
    calendar_id             VARCHAR(36)  NOT NULL,
    place_id                VARCHAR(36)  NOT NULL,
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
    locked                  BOOLEAN      NOT NULL DEFAULT FALSE,
    updated_at              DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (calendar_id, place_id, year, month, day),
    KEY idx_cal_place_weather_month (calendar_id, year, month),
    CONSTRAINT fk_cal_place_weather_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    CONSTRAINT fk_cal_place_weather_place FOREIGN KEY (place_id) REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS calendar_party_places (
    calendar_id VARCHAR(36) NOT NULL,
    year        INT         NOT NULL,
    month       INT         NOT NULL,
    day         INT         NOT NULL,
    place_id    VARCHAR(36) DEFAULT NULL,
    updated_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (calendar_id, year, month, day),
    CONSTRAINT fk_cal_party_places_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    CONSTRAINT fk_cal_party_places_place FOREIGN KEY (place_id) REFERENCES entities(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
