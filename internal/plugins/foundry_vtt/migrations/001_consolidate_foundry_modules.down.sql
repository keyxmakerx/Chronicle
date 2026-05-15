-- C-FMC-5c down migration: reverses 001's rename and re-creates the
-- foundry_module_versions table from its original (post-PR-303) schema.
--
-- WARNING: data in foundry_module_versions cannot be recovered — the up
-- migration verifies the table is empty before dropping, so the down
-- recreates an empty table with the same schema. Rolling back is only
-- meaningful for restoring foundry_modules' code; it does NOT restore
-- any catalog data that was somehow present at up-migration time.
--
-- The schema below combines original 001 (foundry_module_versions table)
-- + 002 (source ENUM + github_release_* columns + uploaded_by_user_id
-- relaxed to NULL). If this file is run, the operator is mid-rollback;
-- the existing foundry_modules plugin code expects this exact shape.

CREATE TABLE IF NOT EXISTS foundry_module_versions (
    id                      CHAR(36)     NOT NULL,
    version                 VARCHAR(50)  NOT NULL,
    file_path               VARCHAR(500) NOT NULL,
    file_size               BIGINT       NOT NULL,
    content_sha256          CHAR(64)     NOT NULL,
    manifest_json           LONGTEXT     NOT NULL,
    compatibility_minimum   VARCHAR(20),
    compatibility_verified  VARCHAR(20),
    compatibility_maximum   VARCHAR(20),
    status                  ENUM('available', 'deprecated', 'withdrawn') NOT NULL DEFAULT 'available',
    release_notes           TEXT,
    uploaded_by_user_id     CHAR(36)     NULL,
    uploaded_at             TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at              TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    source                  ENUM('manual_upload', 'github_release') NOT NULL DEFAULT 'manual_upload',
    github_release_tag      VARCHAR(50)  NULL,
    github_release_id       BIGINT       NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_foundry_module_version (version),
    UNIQUE KEY uk_github_release (github_release_id),
    KEY idx_foundry_module_status (status),
    KEY idx_foundry_module_uploaded_at (uploaded_at),
    CONSTRAINT fk_fmv_uploader FOREIGN KEY (uploaded_by_user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

RENAME TABLE foundry_vtt_campaign_tokens TO foundry_module_campaign_tokens;
