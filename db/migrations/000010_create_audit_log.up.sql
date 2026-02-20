CREATE TABLE IF NOT EXISTS audit_log (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    campaign_id CHAR(36) NOT NULL,
    user_id     CHAR(36) NOT NULL,
    action      VARCHAR(50) NOT NULL,
    entity_type VARCHAR(50) NOT NULL DEFAULT '',
    entity_id   VARCHAR(36) NOT NULL DEFAULT '',
    entity_name VARCHAR(255) NOT NULL DEFAULT '',
    details     JSON DEFAULT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_audit_campaign_created (campaign_id, created_at DESC),
    INDEX idx_audit_entity (entity_id),
    INDEX idx_audit_user (user_id),

    CONSTRAINT fk_audit_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_audit_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
