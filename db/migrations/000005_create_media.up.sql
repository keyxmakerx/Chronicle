-- Migration: 000005_create_media
-- Description: Creates media_files table for file uploads (images) and adds
--              backdrop_path column to campaigns for header images.

CREATE TABLE IF NOT EXISTS media_files (
    id          CHAR(36) NOT NULL PRIMARY KEY,
    campaign_id CHAR(36) NULL,
    uploaded_by CHAR(36) NOT NULL,
    filename    VARCHAR(500) NOT NULL,
    original_name VARCHAR(500) NOT NULL,
    mime_type   VARCHAR(100) NOT NULL,
    file_size   BIGINT NOT NULL,
    usage_type  VARCHAR(50) NOT NULL DEFAULT 'attachment',
    thumbnail_paths JSON DEFAULT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_media_campaign (campaign_id),
    INDEX idx_media_uploaded_by (uploaded_by),

    CONSTRAINT fk_media_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE SET NULL,
    CONSTRAINT fk_media_user FOREIGN KEY (uploaded_by) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE campaigns ADD COLUMN backdrop_path VARCHAR(500) DEFAULT NULL AFTER settings;
