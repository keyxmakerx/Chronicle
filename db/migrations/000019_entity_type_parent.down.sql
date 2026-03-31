ALTER TABLE entity_types DROP FOREIGN KEY fk_entity_types_parent;
DROP INDEX idx_entity_types_parent ON entity_types;
ALTER TABLE entity_types DROP COLUMN parent_type_id;
