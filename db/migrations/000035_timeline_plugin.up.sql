-- Timeline plugin: interactive visual timelines with zoom levels and entity grouping.
-- Each campaign can have multiple timelines, each referencing a calendar.
-- Timeline events are links to existing calendar events (many-to-many).
-- Entity groups provide swim-lane organization for the visualization.

-- Timeline definitions. Each timeline references a specific calendar and can be
-- hidden per-player via visibility_rules JSON.
CREATE TABLE IF NOT EXISTS timelines (
    id               VARCHAR(36)  NOT NULL PRIMARY KEY,
    campaign_id      VARCHAR(36)  NOT NULL,
    calendar_id      VARCHAR(36)  NOT NULL,
    name             VARCHAR(255) NOT NULL,
    description      TEXT,
    description_html TEXT,
    color            VARCHAR(7)   NOT NULL DEFAULT '#6366f1',
    icon             VARCHAR(100) NOT NULL DEFAULT 'fa-timeline',
    visibility       VARCHAR(20)  NOT NULL DEFAULT 'everyone',
    visibility_rules JSON         DEFAULT NULL,
    sort_order       INT          NOT NULL DEFAULT 0,
    zoom_default     VARCHAR(20)  NOT NULL DEFAULT 'year',
    created_by       VARCHAR(36)  DEFAULT NULL,
    created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_timelines_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_timelines_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_timelines_campaign (campaign_id, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Join table linking calendar events to timelines (many-to-many).
-- Supports per-event overrides for label, color, visibility.
CREATE TABLE IF NOT EXISTS timeline_event_links (
    id                  INT          AUTO_INCREMENT PRIMARY KEY,
    timeline_id         VARCHAR(36)  NOT NULL,
    event_id            VARCHAR(36)  NOT NULL,
    display_order       INT          NOT NULL DEFAULT 0,
    visibility_override VARCHAR(20)  DEFAULT NULL,
    visibility_rules    JSON         DEFAULT NULL,
    label               VARCHAR(255) DEFAULT NULL,
    color_override      VARCHAR(7)   DEFAULT NULL,
    created_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_tel_timeline FOREIGN KEY (timeline_id) REFERENCES timelines(id) ON DELETE CASCADE,
    CONSTRAINT fk_tel_event FOREIGN KEY (event_id) REFERENCES calendar_events(id) ON DELETE CASCADE,
    UNIQUE KEY uq_timeline_event (timeline_id, event_id),
    INDEX idx_tel_timeline (timeline_id, display_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Named entity groups for swim-lane organization on the timeline visualization.
CREATE TABLE IF NOT EXISTS timeline_entity_groups (
    id          INT          AUTO_INCREMENT PRIMARY KEY,
    timeline_id VARCHAR(36)  NOT NULL,
    name        VARCHAR(200) NOT NULL,
    color       VARCHAR(7)   NOT NULL DEFAULT '#6b7280',
    sort_order  INT          NOT NULL DEFAULT 0,
    CONSTRAINT fk_teg_timeline FOREIGN KEY (timeline_id) REFERENCES timelines(id) ON DELETE CASCADE,
    INDEX idx_teg_timeline (timeline_id, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Members of entity groups. Each entity can belong to one group per timeline.
CREATE TABLE IF NOT EXISTS timeline_entity_group_members (
    id        INT         AUTO_INCREMENT PRIMARY KEY,
    group_id  INT         NOT NULL,
    entity_id VARCHAR(36) NOT NULL,
    CONSTRAINT fk_tegm_group FOREIGN KEY (group_id) REFERENCES timeline_entity_groups(id) ON DELETE CASCADE,
    CONSTRAINT fk_tegm_entity FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE,
    UNIQUE KEY uq_group_entity (group_id, entity_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Update the timeline addon from planned/widget to active/plugin now that
-- the backing code is implemented. Row was seeded in migration 000015.
-- The 'plugin' ENUM value was added in migration 000027.
UPDATE addons
SET category    = 'plugin',
    status      = 'active',
    description = 'Interactive visual timelines with zoom levels, entity grouping, and calendar integration. Multiple timelines per campaign.'
WHERE slug = 'timeline';
