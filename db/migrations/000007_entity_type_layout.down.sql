-- Rollback: 000007_entity_type_layout

ALTER TABLE entity_types DROP COLUMN IF EXISTS layout_json;
