-- Rollback: 000021_add_dashboard_layout
-- Removes dashboard_layout columns from campaigns and entity_types.

ALTER TABLE campaigns DROP COLUMN IF EXISTS dashboard_layout;
ALTER TABLE entity_types DROP COLUMN IF EXISTS dashboard_layout;
