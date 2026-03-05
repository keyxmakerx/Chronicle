-- Campaign groups allow owners to organize members into named groups
-- for use in per-entity permission grants (e.g., "only the rogue's party
-- can see this entity").

CREATE TABLE campaign_groups (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    campaign_id CHAR(36)     NOT NULL,
    name        VARCHAR(100) NOT NULL,
    description VARCHAR(500) DEFAULT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_campaign_groups_campaign (campaign_id),
    UNIQUE KEY uq_campaign_group_name (campaign_id, name),

    CONSTRAINT fk_campaign_groups_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE campaign_group_members (
    group_id   INT      NOT NULL,
    user_id    CHAR(36) NOT NULL,
    joined_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (group_id, user_id),
    INDEX idx_group_members_user (user_id),

    CONSTRAINT fk_group_members_group
        FOREIGN KEY (group_id) REFERENCES campaign_groups(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Add 'group' to the subject_type ENUM in entity_permissions.
ALTER TABLE entity_permissions
    MODIFY COLUMN subject_type ENUM('role','user','group') NOT NULL;
