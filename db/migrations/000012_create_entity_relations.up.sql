-- Migration 000012: Create entity_relations table.
-- Entity relations enable bi-directional linking between entities within a campaign.
-- Each row stores one direction of a relation (A→B); the service layer creates
-- the reverse direction (B→A) automatically when a relation is created.

CREATE TABLE entity_relations (
    id                    INT AUTO_INCREMENT PRIMARY KEY,
    campaign_id           VARCHAR(36) NOT NULL,
    source_entity_id      VARCHAR(36) NOT NULL,
    target_entity_id      VARCHAR(36) NOT NULL,
    relation_type         VARCHAR(100) NOT NULL,
    reverse_relation_type VARCHAR(100) NOT NULL,
    created_at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by            VARCHAR(36) NOT NULL,

    -- Prevent duplicate relations of the same type between the same pair.
    UNIQUE KEY uq_entity_relations_pair (source_entity_id, target_entity_id, relation_type),

    -- Indexes for efficient lookups by source, target, and campaign.
    INDEX idx_entity_relations_source (source_entity_id),
    INDEX idx_entity_relations_target (target_entity_id),
    INDEX idx_entity_relations_campaign (campaign_id),

    -- Foreign keys: cascade delete when the entity or campaign is removed.
    CONSTRAINT fk_entity_relations_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_relations_source
        FOREIGN KEY (source_entity_id) REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_relations_target
        FOREIGN KEY (target_entity_id) REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_relations_creator
        FOREIGN KEY (created_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
