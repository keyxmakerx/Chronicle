-- Content extension system tables.
-- Extensions are user-installable content packs (calendar presets, entity type
-- templates, entity packs, tag collections, marker icons, themes, reference data).
-- They are uploaded as zip files by site admins and enabled per-campaign by owners.

-- Installed extensions registry (site-wide).
CREATE TABLE extensions (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    ext_id VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    version VARCHAR(20) NOT NULL,
    description TEXT NOT NULL,
    manifest JSON NOT NULL,
    installed_by VARCHAR(36) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT NOW(),
    updated_at DATETIME NOT NULL DEFAULT NOW() ON UPDATE NOW(),
    CONSTRAINT fk_ext_installed_by FOREIGN KEY (installed_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Per-campaign extension activation.
CREATE TABLE campaign_extensions (
    campaign_id VARCHAR(36) NOT NULL,
    extension_id VARCHAR(36) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    applied_contents JSON DEFAULT '{}',
    enabled_at DATETIME NOT NULL DEFAULT NOW(),
    enabled_by VARCHAR(36) NULL,
    PRIMARY KEY (campaign_id, extension_id),
    CONSTRAINT fk_ce_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_ce_extension FOREIGN KEY (extension_id) REFERENCES extensions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Provenance tracking: which extension created which records.
-- Enables clean uninstall and "provided by extension X" badges.
CREATE TABLE extension_provenance (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    campaign_id VARCHAR(36) NOT NULL,
    extension_id VARCHAR(36) NOT NULL,
    table_name VARCHAR(64) NOT NULL,
    record_id VARCHAR(36) NOT NULL,
    record_type VARCHAR(50) NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT NOW(),
    INDEX idx_ep_campaign_ext (campaign_id, extension_id),
    INDEX idx_ep_table_record (table_name, record_id),
    CONSTRAINT fk_ep_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_ep_extension FOREIGN KEY (extension_id) REFERENCES extensions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Extension-specific data that doesn't fit existing tables.
-- Used for relation type suggestions, marker icon metadata, etc.
CREATE TABLE extension_data (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    campaign_id VARCHAR(36) NOT NULL,
    extension_id VARCHAR(36) NOT NULL,
    namespace VARCHAR(50) NOT NULL,
    data_key VARCHAR(100) NOT NULL,
    data_value JSON NOT NULL,
    UNIQUE KEY uk_ext_data (campaign_id, extension_id, namespace, data_key),
    CONSTRAINT fk_ed_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_ed_extension FOREIGN KEY (extension_id) REFERENCES extensions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
