-- Migration 000004: Create entity_types and entities tables.
-- Entity types are configurable per campaign with JSON field definitions.
-- Entities are the worldbuilding objects (characters, locations, items, etc.).

CREATE TABLE entity_types (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    campaign_id CHAR(36) NOT NULL,
    slug       VARCHAR(100) NOT NULL,
    name       VARCHAR(100) NOT NULL,
    name_plural VARCHAR(100) NOT NULL,
    icon       VARCHAR(50) NOT NULL DEFAULT 'fa-file',
    color      VARCHAR(7) NOT NULL DEFAULT '#6b7280',
    fields     JSON NOT NULL DEFAULT ('[]'),
    sort_order INT NOT NULL DEFAULT 0,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,

    UNIQUE KEY uq_entity_types_campaign_slug (campaign_id, slug),
    CONSTRAINT fk_entity_types_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE entities (
    id             CHAR(36) PRIMARY KEY,
    campaign_id    CHAR(36) NOT NULL,
    entity_type_id INT NOT NULL,
    name           VARCHAR(200) NOT NULL,
    slug           VARCHAR(200) NOT NULL,
    entry          JSON NULL,
    entry_html     LONGTEXT NULL,
    image_path     VARCHAR(500) NULL,
    parent_id      CHAR(36) NULL,
    type_label     VARCHAR(100) NULL,
    is_private     BOOLEAN NOT NULL DEFAULT FALSE,
    is_template    BOOLEAN NOT NULL DEFAULT FALSE,
    fields_data    JSON NOT NULL DEFAULT ('{}'),
    created_by     CHAR(36) NOT NULL,
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,

    UNIQUE KEY uq_entities_campaign_slug (campaign_id, slug),
    FULLTEXT KEY ft_entities_name (name),
    INDEX idx_entities_campaign_type (campaign_id, entity_type_id),
    INDEX idx_entities_parent (parent_id),

    CONSTRAINT fk_entities_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_entities_type
        FOREIGN KEY (entity_type_id) REFERENCES entity_types(id) ON DELETE CASCADE,
    CONSTRAINT fk_entities_parent
        FOREIGN KEY (parent_id) REFERENCES entities(id) ON DELETE SET NULL,
    CONSTRAINT fk_entities_creator
        FOREIGN KEY (created_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
