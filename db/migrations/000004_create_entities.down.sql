-- Rollback migration 000004: Drop entities and entity_types tables.
-- Order matters: entities references entity_types, so drop it first.

DROP TABLE IF EXISTS entities;
DROP TABLE IF EXISTS entity_types;
