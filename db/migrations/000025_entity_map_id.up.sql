-- Adds `entities.map_id` as a nullable reference to a campaign map. This
-- powers the per-entity Map Editor block (e.g. a Location entity that
-- "is" a Map). The column is added here without the foreign-key
-- constraint because `maps` is a plugin table — core migrations run
-- before plugin migrations and would fail on a fresh DB. The matching
-- FK is added by the maps plugin's migration 005.
--
-- Index supports the dominant query: "find all entities pointing at
-- this map" (used by the heal logic when a map is deleted, and by
-- future audit dashboards).
ALTER TABLE entities
    ADD COLUMN map_id CHAR(36) NULL AFTER owner_user_id,
    ADD INDEX idx_entities_map_id (map_id);
