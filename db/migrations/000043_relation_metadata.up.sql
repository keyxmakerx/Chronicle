-- Migration 000043: Add metadata column to entity_relations.
-- Enables shop inventory (price, quantity, stock status) and other
-- relation-specific data without creating separate tables.

ALTER TABLE entity_relations
    ADD COLUMN metadata JSON COMMENT 'Relation-specific data (e.g., shop: price, quantity)';
