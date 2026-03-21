-- Layout presets: reusable page layout configurations that can be applied to
-- any entity type. Campaign-scoped with optional built-in presets seeded on
-- campaign creation.
CREATE TABLE IF NOT EXISTS layout_presets (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    campaign_id CHAR(36) NOT NULL,
    name       VARCHAR(200) NOT NULL,
    description VARCHAR(500) DEFAULT '',
    layout_json JSON NOT NULL,
    icon       VARCHAR(50) DEFAULT 'fa-table-columns',
    sort_order INT DEFAULT 0,
    is_builtin BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_lp_campaign (campaign_id),
    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
