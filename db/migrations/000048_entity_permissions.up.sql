-- entity_permissions: Per-entity access control beyond the simple is_private flag.
-- subject_type can be 'role' (campaign-wide role threshold) or 'user' (individual grant).
-- permission can be 'view' or 'edit'.
-- When an entity has rows in this table, the permission system switches from
-- legacy is_private mode to fine-grained mode for that entity.

CREATE TABLE IF NOT EXISTS entity_permissions (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    entity_id   CHAR(36)    NOT NULL,
    subject_type ENUM('role','user') NOT NULL,
    subject_id  VARCHAR(36) NOT NULL,  -- role int as string ('1','2','3') or user UUID
    permission  ENUM('view','edit') NOT NULL,

    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE KEY uq_entity_perm (entity_id, subject_type, subject_id, permission),
    INDEX idx_entity_perm_entity (entity_id),
    INDEX idx_entity_perm_subject (subject_type, subject_id),

    CONSTRAINT fk_entity_perm_entity
        FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Add visibility mode column to entities: 'default' uses is_private legacy,
-- 'custom' uses entity_permissions table.
ALTER TABLE entities
    ADD COLUMN visibility ENUM('default','custom') NOT NULL DEFAULT 'default' AFTER is_private;
