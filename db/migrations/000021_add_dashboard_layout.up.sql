-- Migration: 000021_add_dashboard_layout
-- Description: Adds dashboard_layout JSON column to campaigns and entity_types
-- tables. Stores configurable dashboard block layouts as JSON (row/column/block
-- structure). NULL means "use the hardcoded default layout" for backwards compat.

ALTER TABLE campaigns
    ADD COLUMN dashboard_layout JSON DEFAULT NULL AFTER sidebar_config;

ALTER TABLE entity_types
    ADD COLUMN dashboard_layout JSON DEFAULT NULL AFTER pinned_entity_ids;
