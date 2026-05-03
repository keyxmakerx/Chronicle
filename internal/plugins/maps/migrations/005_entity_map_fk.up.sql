-- Wires the FK between entities.map_id (added in core migration 025)
-- and maps.id. ON DELETE SET NULL is the right policy: if a map gets
-- deleted, every entity that pointed at it gracefully degrades to "no
-- map assigned" (the renderer falls back to the picker for DM/Scribe,
-- empty state for players) instead of leaving dangling FK references
-- or refusing the delete.
--
-- Lives in the maps plugin (not core) because `maps` is a plugin
-- table; core migrations cannot reference it without crashing on a
-- fresh DB. See ADR / `.ai/conventions.md` "Migration Safety Rules".
ALTER TABLE entities
    ADD CONSTRAINT fk_entities_map_id
        FOREIGN KEY (map_id) REFERENCES maps(id)
        ON DELETE SET NULL;
