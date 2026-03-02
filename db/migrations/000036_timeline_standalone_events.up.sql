-- Allow timelines without a calendar (standalone events only).
ALTER TABLE timelines DROP FOREIGN KEY fk_timelines_calendar;
ALTER TABLE timelines MODIFY calendar_id VARCHAR(36) DEFAULT NULL;
ALTER TABLE timelines ADD CONSTRAINT fk_timelines_calendar
    FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE SET NULL;

-- Standalone timeline events belong directly to a timeline (1:N).
-- Full parity with calendar_events fields: multi-day spans, times, recurrence.
CREATE TABLE IF NOT EXISTS timeline_events (
    id               VARCHAR(36)  NOT NULL PRIMARY KEY,
    timeline_id      VARCHAR(36)  NOT NULL,
    entity_id        VARCHAR(36)  DEFAULT NULL,
    name             VARCHAR(255) NOT NULL,
    description      TEXT,
    description_html TEXT         DEFAULT NULL,
    year             INT          NOT NULL,
    month            INT          NOT NULL DEFAULT 1,
    day              INT          NOT NULL DEFAULT 1,
    start_hour       INT          DEFAULT NULL,
    start_minute     INT          DEFAULT NULL,
    end_year         INT          DEFAULT NULL,
    end_month        INT          DEFAULT NULL,
    end_day          INT          DEFAULT NULL,
    end_hour         INT          DEFAULT NULL,
    end_minute       INT          DEFAULT NULL,
    is_recurring     TINYINT(1)   NOT NULL DEFAULT 0,
    recurrence_type  VARCHAR(20)  DEFAULT NULL,
    category         VARCHAR(100) DEFAULT NULL,
    visibility       VARCHAR(20)  NOT NULL DEFAULT 'everyone',
    display_order    INT          NOT NULL DEFAULT 0,
    label            VARCHAR(255) DEFAULT NULL,
    color            VARCHAR(7)   DEFAULT NULL,
    created_by       VARCHAR(36)  DEFAULT NULL,
    created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_te_timeline FOREIGN KEY (timeline_id) REFERENCES timelines(id) ON DELETE CASCADE,
    CONSTRAINT fk_te_entity   FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE SET NULL,
    INDEX idx_te_timeline_date (timeline_id, year, month, day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
