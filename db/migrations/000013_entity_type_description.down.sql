ALTER TABLE entity_types
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS pinned_entity_ids;
