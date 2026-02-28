-- Calendar plugin: custom calendars with non-Gregorian months, moons, eras,
-- and events linked to entities. Each campaign can have one calendar.

-- Core calendar definition with current in-game date tracking.
CREATE TABLE IF NOT EXISTS calendars (
    id          VARCHAR(36)  NOT NULL PRIMARY KEY,
    campaign_id VARCHAR(36)  NOT NULL,
    name        VARCHAR(255) NOT NULL DEFAULT 'Campaign Calendar',
    description TEXT,
    epoch_name  VARCHAR(100) DEFAULT NULL,
    current_year INT         NOT NULL DEFAULT 1,
    current_month INT        NOT NULL DEFAULT 1,
    current_day  INT         NOT NULL DEFAULT 1,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY idx_calendars_campaign (campaign_id),
    CONSTRAINT fk_calendars_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Named months with configurable day counts. sort_order defines month sequence.
-- is_intercalary marks leap/festival months that don't appear in normal weekday rotation.
CREATE TABLE IF NOT EXISTS calendar_months (
    id          INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    calendar_id VARCHAR(36)  NOT NULL,
    name        VARCHAR(100) NOT NULL,
    days        INT          NOT NULL DEFAULT 30,
    sort_order  INT          NOT NULL DEFAULT 0,
    is_intercalary TINYINT(1) NOT NULL DEFAULT 0,
    CONSTRAINT fk_cal_months_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_cal_months_order (calendar_id, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Named weekdays in repeating cycle. sort_order defines the sequence.
CREATE TABLE IF NOT EXISTS calendar_weekdays (
    id          INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    calendar_id VARCHAR(36)  NOT NULL,
    name        VARCHAR(100) NOT NULL,
    sort_order  INT          NOT NULL DEFAULT 0,
    CONSTRAINT fk_cal_weekdays_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_cal_weekdays_order (calendar_id, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Moons with cycle length and phase offset for moon phase calculations.
CREATE TABLE IF NOT EXISTS calendar_moons (
    id           INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    calendar_id  VARCHAR(36)  NOT NULL,
    name         VARCHAR(100) NOT NULL,
    cycle_days   FLOAT        NOT NULL DEFAULT 29.5,
    phase_offset FLOAT        NOT NULL DEFAULT 0,
    color        VARCHAR(7)   NOT NULL DEFAULT '#c0c0c0',
    CONSTRAINT fk_cal_moons_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_cal_moons_calendar (calendar_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Named seasons with start/end month+day ranges.
CREATE TABLE IF NOT EXISTS calendar_seasons (
    id          INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    calendar_id VARCHAR(36)  NOT NULL,
    name        VARCHAR(100) NOT NULL,
    start_month INT          NOT NULL,
    start_day   INT          NOT NULL,
    end_month   INT          NOT NULL,
    end_day     INT          NOT NULL,
    description TEXT,
    CONSTRAINT fk_cal_seasons_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_cal_seasons_calendar (calendar_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Calendar events linked to optional entities. Supports recurring events.
-- visibility: 'everyone' or 'dm_only' (matches entity privacy model).
CREATE TABLE IF NOT EXISTS calendar_events (
    id              VARCHAR(36)  NOT NULL PRIMARY KEY,
    calendar_id     VARCHAR(36)  NOT NULL,
    entity_id       VARCHAR(36)  DEFAULT NULL,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    year            INT          NOT NULL,
    month           INT          NOT NULL,
    day             INT          NOT NULL,
    is_recurring    TINYINT(1)   NOT NULL DEFAULT 0,
    recurrence_type VARCHAR(20)  DEFAULT NULL,
    visibility      VARCHAR(20)  NOT NULL DEFAULT 'everyone',
    created_by      VARCHAR(36)  DEFAULT NULL,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_cal_events_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    CONSTRAINT fk_cal_events_entity FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE SET NULL,
    INDEX idx_cal_events_date (calendar_id, year, month, day),
    INDEX idx_cal_events_entity (entity_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Register the calendar addon so campaign owners can enable it.
INSERT INTO addons (slug, name, description, version, category, status, icon, author)
VALUES ('calendar', 'Calendar', 'Custom fantasy calendar with configurable months, weekdays, moons, seasons, and events. Link events to entities for timeline tracking.', '0.1.0', 'plugin', 'active', 'fa-calendar-days', 'Chronicle');
