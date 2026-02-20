-- Notes: per-user personal notes scoped to campaigns and optionally entities.
-- Powers the floating notes panel (Google Keep-style) with checklists.
CREATE TABLE IF NOT EXISTS notes (
    id          CHAR(36) PRIMARY KEY,
    campaign_id CHAR(36) NOT NULL,
    user_id     CHAR(36) NOT NULL,
    entity_id   CHAR(36) DEFAULT NULL,       -- NULL = campaign-wide note, set = page-specific
    title       VARCHAR(200) NOT NULL DEFAULT 'Untitled',
    content     JSON NOT NULL,               -- Array of blocks: [{type, value/items}]
    color       VARCHAR(20) NOT NULL DEFAULT '#374151',
    pinned      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_notes_user_campaign (user_id, campaign_id),
    INDEX idx_notes_entity (entity_id),
    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
