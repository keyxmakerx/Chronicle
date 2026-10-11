-- 010_marker_linked_map (down): drop the pin-to-map link. Destroys every stored
-- link; the pins themselves stay as plain pins. Idempotent.
ALTER TABLE map_markers
    DROP FOREIGN KEY IF EXISTS fk_markers_linked_map;

ALTER TABLE map_markers
    DROP INDEX IF EXISTS idx_markers_linked_map,
    DROP COLUMN IF EXISTS linked_map_id;
