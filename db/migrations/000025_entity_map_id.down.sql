-- Drop the FK first (lives in maps plugin migration 005) — its down
-- migration handles that. This file just drops the column + index.
ALTER TABLE entities
    DROP INDEX idx_entities_map_id,
    DROP COLUMN map_id;
