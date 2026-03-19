-- Restore is_folder column.
ALTER TABLE entities ADD COLUMN is_folder BOOLEAN NOT NULL DEFAULT FALSE AFTER sort_order;

-- Move sidebar_nodes back to entities with is_folder=TRUE.
INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, sort_order, is_folder, created_by, created_at, updated_at)
SELECT sn.id, sn.campaign_id, sn.entity_type_id, sn.name, LOWER(REPLACE(sn.name, ' ', '-')), sn.sort_order, TRUE,
       (SELECT id FROM users LIMIT 1), sn.created_at, sn.created_at
FROM sidebar_nodes sn;

-- Move entities with parent_node_id back to parent_id.
UPDATE entities SET parent_id = parent_node_id, parent_node_id = NULL
WHERE parent_node_id IS NOT NULL;

-- Drop parent_node_id and sidebar_nodes.
ALTER TABLE entities DROP FOREIGN KEY fk_entities_parent_node;
ALTER TABLE entities DROP INDEX idx_entities_parent_node;
ALTER TABLE entities DROP COLUMN parent_node_id;
DROP TABLE sidebar_nodes;
