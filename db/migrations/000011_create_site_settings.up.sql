CREATE TABLE IF NOT EXISTS site_settings (
    setting_key   VARCHAR(100) NOT NULL PRIMARY KEY,
    setting_value TEXT NOT NULL,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Default storage limits (editable by admin).
INSERT INTO site_settings (setting_key, setting_value) VALUES
    ('storage.max_upload_size', '10485760'),
    ('storage.max_storage_per_user', '0'),
    ('storage.max_storage_per_campaign', '0'),
    ('storage.max_files_per_campaign', '0'),
    ('storage.rate_limit_uploads_per_min', '30');

-- Per-user storage overrides.
CREATE TABLE IF NOT EXISTS user_storage_limits (
    user_id              CHAR(36) NOT NULL PRIMARY KEY,
    max_upload_size      BIGINT DEFAULT NULL,
    max_total_storage    BIGINT DEFAULT NULL,
    updated_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    CONSTRAINT fk_user_storage_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Per-campaign storage overrides.
CREATE TABLE IF NOT EXISTS campaign_storage_limits (
    campaign_id          CHAR(36) NOT NULL PRIMARY KEY,
    max_total_storage    BIGINT DEFAULT NULL,
    max_files            INT DEFAULT NULL,
    updated_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    CONSTRAINT fk_campaign_storage_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
