-- Sidebar nodes provide pure organizational folders in the sidebar tree.
-- Unlike entities, folder nodes have no page content — they exist solely
-- for hierarchical grouping within a category's entity tree.
CREATE TABLE sidebar_nodes (
  id             CHAR(36)     NOT NULL PRIMARY KEY,
  campaign_id    CHAR(36)     NOT NULL,
  entity_type_id INT          NOT NULL,
  name           VARCHAR(255) NOT NULL,
  parent_id      CHAR(36)     NULL,
  sort_order     INT          NOT NULL DEFAULT 0,
  node_type      ENUM('folder') NOT NULL DEFAULT 'folder',
  created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
  FOREIGN KEY (entity_type_id) REFERENCES entity_types(id) ON DELETE CASCADE,
  INDEX idx_sidebar_nodes_campaign (campaign_id, entity_type_id),
  INDEX idx_sidebar_nodes_parent (parent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Allow entities to be parented under a sidebar folder node.
-- An entity's parent is either parent_id (another entity) OR
-- parent_node_id (a folder node), never both.
ALTER TABLE entities ADD COLUMN parent_node_id CHAR(36) NULL AFTER parent_id;
ALTER TABLE entities ADD INDEX idx_entities_parent_node (parent_node_id);
ALTER TABLE entities ADD FOREIGN KEY fk_entities_parent_node (parent_node_id)
  REFERENCES sidebar_nodes(id) ON DELETE SET NULL;

-- Migrate existing is_folder entities to sidebar_nodes.
INSERT INTO sidebar_nodes (id, campaign_id, entity_type_id, name, sort_order, node_type, created_at)
SELECT id, campaign_id, entity_type_id, name, sort_order, 'folder', created_at
FROM entities
WHERE is_folder = TRUE;

-- Reparent children of is_folder entities to use parent_node_id instead.
UPDATE entities e
INNER JOIN entities folder ON e.parent_id = folder.id AND folder.is_folder = TRUE
SET e.parent_node_id = folder.id, e.parent_id = NULL;

-- Delete the migrated is_folder entities.
DELETE FROM entities WHERE is_folder = TRUE;

-- Drop the is_folder column now that data is migrated.
ALTER TABLE entities DROP COLUMN is_folder;
