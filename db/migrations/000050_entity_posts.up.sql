-- entity_posts: sub-notes/posts attached to entities.
-- Each post is a named content section below the main entity entry.
-- Posts support TipTap rich text, per-post visibility, and drag-to-reorder.
CREATE TABLE IF NOT EXISTS entity_posts (
    id          CHAR(36)     NOT NULL,
    entity_id   CHAR(36)     NOT NULL,
    campaign_id CHAR(36)     NOT NULL,
    name        VARCHAR(200) NOT NULL,
    entry       JSON         NULL COMMENT 'TipTap/ProseMirror JSON',
    entry_html  LONGTEXT     NULL COMMENT 'Pre-rendered HTML for display',
    is_private  BOOLEAN      NOT NULL DEFAULT FALSE COMMENT 'DM-only post',
    sort_order  INT          NOT NULL DEFAULT 0,
    created_by  CHAR(36)     NOT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    INDEX idx_entity_posts_entity (entity_id),
    INDEX idx_entity_posts_campaign (campaign_id),
    CONSTRAINT fk_entity_posts_entity FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_posts_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
