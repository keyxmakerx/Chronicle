-- 011_map_trash (down): maps still in the Trash come back into use, so rolling
-- back never deletes a map. Idempotent.
DROP INDEX IF EXISTS idx_maps_deleted_at ON maps;
ALTER TABLE maps DROP COLUMN IF EXISTS deleted_at;
