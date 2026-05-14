-- foundry_module_versions catalogues each version of the Foundry VTT module
-- that an admin has uploaded to this Chronicle server. The catalog is the
-- single source of truth for "which versions exist" — Foundry no longer
-- pulls from GitHub. file_path points at a plugin-managed location on disk
-- (not the media plugin) because module zips are server-global assets,
-- not campaign-scoped media, and don't need the dedup/quota machinery.
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
    -- 'available' is the install-able state; 'deprecated' still lets
    -- existing pins resolve but warns the owner; 'withdrawn' returns
    -- 404 on the manifest endpoint and disappears from the selectable
    -- list so a compromised version can be revoked without dropping
    -- the row.
    status                  ENUM('available', 'deprecated', 'withdrawn') NOT NULL DEFAULT 'available',
    release_notes           TEXT,
    uploaded_by_user_id     CHAR(36)     NOT NULL,
    uploaded_at             TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at              TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_foundry_module_version (version),
    KEY idx_foundry_module_status (status),
    KEY idx_foundry_module_uploaded_at (uploaded_at),
    CONSTRAINT fk_fmv_uploader FOREIGN KEY (uploaded_by_user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- foundry_module_campaign_tokens stores the per-campaign signing version
-- for the manifest URL. The actual token string is HMAC-signed and not
-- stored — only the version counter is. Rotation = increment the counter,
-- which invalidates every previously-issued URL for this campaign.
CREATE TABLE IF NOT EXISTS foundry_module_campaign_tokens (
    campaign_id    CHAR(36)  NOT NULL,
    token_version  INT       NOT NULL DEFAULT 1,
    rotated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id),
    CONSTRAINT fk_fmct_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
