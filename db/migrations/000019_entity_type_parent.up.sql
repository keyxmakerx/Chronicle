-- Add parent_type_id to entity_types for type hierarchy (sub-types).
-- A child type (e.g., "NPC") belongs to a parent type (e.g., "Character").
-- ON DELETE SET NULL: if parent is deleted, children become top-level types.

ALTER TABLE entity_types
  ADD COLUMN parent_type_id INT DEFAULT NULL AFTER preset_category,
  ADD CONSTRAINT fk_entity_types_parent
    FOREIGN KEY (parent_type_id) REFERENCES entity_types(id) ON DELETE SET NULL;

CREATE INDEX idx_entity_types_parent ON entity_types (parent_type_id);
