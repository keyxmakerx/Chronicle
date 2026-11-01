-- deleted_at puts a map, with its pins, drawings, tokens and fog, in the
-- campaign Trash. Nothing is removed until the Trash's clean-up deletes the
-- map row and the foreign keys cascade; until then every map read leaves a
-- trashed map out and Restore is one update.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE maps
    ADD COLUMN IF NOT EXISTS deleted_at DATETIME NULL DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_maps_deleted_at ON maps (deleted_at);
