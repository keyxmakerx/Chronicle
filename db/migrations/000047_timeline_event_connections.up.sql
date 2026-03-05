-- Timeline event connections: visual lines/arrows between related events
-- on the timeline visualization. Connects any two events (calendar or
-- standalone) within the same timeline.
CREATE TABLE IF NOT EXISTS timeline_event_connections (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    timeline_id VARCHAR(36) NOT NULL,
    source_id   VARCHAR(36) NOT NULL COMMENT 'Event ID of the source (arrow start)',
    target_id   VARCHAR(36) NOT NULL COMMENT 'Event ID of the target (arrow end)',
    source_type VARCHAR(20) NOT NULL DEFAULT 'standalone' COMMENT 'calendar or standalone',
    target_type VARCHAR(20) NOT NULL DEFAULT 'standalone' COMMENT 'calendar or standalone',
    label       VARCHAR(200) DEFAULT NULL COMMENT 'Optional label displayed on the connection line',
    color       VARCHAR(7)   DEFAULT NULL COMMENT 'Hex color override for the connection line',
    style       VARCHAR(20)  NOT NULL DEFAULT 'arrow' COMMENT 'Line style: arrow, dashed, dotted, solid',
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_tec_timeline FOREIGN KEY (timeline_id) REFERENCES timelines(id) ON DELETE CASCADE,
    UNIQUE KEY uq_tec_pair (timeline_id, source_id, target_id),
    INDEX idx_tec_timeline (timeline_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
