-- Weather system: stores current weather state per calendar.
-- One row per calendar (UNIQUE on calendar_id). Updated by GM or external sync tools.
CREATE TABLE IF NOT EXISTS calendar_weather (
    id                      INT          AUTO_INCREMENT PRIMARY KEY,
    calendar_id             VARCHAR(36)  NOT NULL UNIQUE,
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
    updated_at              DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_cal_weather FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Cycles: periodic named cycles (zodiac, elemental, etc.).
CREATE TABLE IF NOT EXISTS calendar_cycles (
    id           INT          AUTO_INCREMENT PRIMARY KEY,
    calendar_id  VARCHAR(36)  NOT NULL,
    name         VARCHAR(100) NOT NULL,
    cycle_length INT          NOT NULL,
    type         VARCHAR(20)  NOT NULL DEFAULT 'yearly',
    sort_order   INT          NOT NULL DEFAULT 0,
    CONSTRAINT fk_cal_cycles FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_cal_cycles (calendar_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Cycle entries: individual entries in a cycle (e.g., zodiac signs).
CREATE TABLE IF NOT EXISTS calendar_cycle_entries (
    id         INT          AUTO_INCREMENT PRIMARY KEY,
    cycle_id   INT          NOT NULL,
    name       VARCHAR(100) NOT NULL,
    icon       VARCHAR(50)  DEFAULT NULL,
    year_offset INT         NOT NULL DEFAULT 0,
    sort_order INT          NOT NULL DEFAULT 0,
    CONSTRAINT fk_cycle_entries FOREIGN KEY (cycle_id) REFERENCES calendar_cycles(id) ON DELETE CASCADE,
    INDEX idx_cycle_entries (cycle_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Festivals: fixed calendar entries (holidays that are part of the calendar
-- structure, not recurring events). month+day for standard dates, after_month
-- for intercalary festivals that fall between months.
CREATE TABLE IF NOT EXISTS calendar_festivals (
    id          INT          AUTO_INCREMENT PRIMARY KEY,
    calendar_id VARCHAR(36)  NOT NULL,
    name        VARCHAR(200) NOT NULL,
    month       INT          DEFAULT NULL,
    day         INT          DEFAULT NULL,
    after_month INT          DEFAULT NULL,
    description TEXT         DEFAULT NULL,
    color       VARCHAR(20)  DEFAULT NULL,
    icon        VARCHAR(50)  DEFAULT NULL,
    sort_order  INT          NOT NULL DEFAULT 0,
    CONSTRAINT fk_cal_festivals FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_cal_festivals (calendar_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
