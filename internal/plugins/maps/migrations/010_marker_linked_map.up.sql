-- A pin can open another map of the same campaign. linked_map_id is that map;
-- NULL is a plain pin. The service checks the target is a map of the pin's own
-- campaign and not the pin's own map; the column cannot express either.
--
-- ON DELETE SET NULL: deleting a map turns every pin that opened it back into
-- a plain pin instead of refusing the delete or removing the pins, the same
-- policy entities.map_id follows (005_entity_map_fk). The index serves the
-- foreign key and the link-tree read, which looks up every pin that opens a map.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE map_markers
    ADD COLUMN IF NOT EXISTS linked_map_id VARCHAR(36) DEFAULT NULL AFTER entity_id;

ALTER TABLE map_markers
    ADD INDEX IF NOT EXISTS idx_markers_linked_map (linked_map_id);

ALTER TABLE map_markers
    ADD CONSTRAINT fk_markers_linked_map
        FOREIGN KEY IF NOT EXISTS (linked_map_id) REFERENCES maps(id) ON DELETE SET NULL;
