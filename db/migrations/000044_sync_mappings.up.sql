-- Migration 000044: sync_mappings table for bidirectional Foundry VTT sync.
-- Tracks the mapping between Chronicle objects and their external counterparts
-- (e.g., Chronicle entity ↔ Foundry JournalEntry) along with version tracking.

CREATE TABLE sync_mappings (
    id CHAR(36) NOT NULL PRIMARY KEY,
    campaign_id CHAR(36) NOT NULL,
    chronicle_type VARCHAR(50) NOT NULL COMMENT 'entity, map, calendar_event, marker, drawing, token',
    chronicle_id CHAR(36) NOT NULL,
    external_system VARCHAR(50) NOT NULL COMMENT 'foundry',
    external_id VARCHAR(255) NOT NULL COMMENT 'Foundry document ID',
    sync_version INT NOT NULL DEFAULT 1,
    last_synced_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    sync_direction VARCHAR(10) NOT NULL DEFAULT 'both' COMMENT 'both, push, pull',
    sync_metadata JSON COMMENT 'Extra sync state (e.g., Foundry document type, page IDs)',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_sync_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    UNIQUE KEY uq_sync_mapping (campaign_id, chronicle_type, chronicle_id, external_system),
    INDEX idx_sync_external (campaign_id, external_system, external_id),
    INDEX idx_sync_type (campaign_id, chronicle_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
