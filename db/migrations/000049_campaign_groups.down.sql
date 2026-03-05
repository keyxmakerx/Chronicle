-- Revert entity_permissions subject_type ENUM to exclude 'group'.
-- First delete any group-type permission rows.
DELETE FROM entity_permissions WHERE subject_type = 'group';
ALTER TABLE entity_permissions
    MODIFY COLUMN subject_type ENUM('role','user') NOT NULL;

DROP TABLE IF EXISTS campaign_group_members;
DROP TABLE IF EXISTS campaign_groups;
