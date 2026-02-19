-- Rollback: 000007_entity_type_layout

ALTER TABLE entity_types DROP COLUMN layout_json;
