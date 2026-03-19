-- Add is_folder flag to entities for organizational containers.
-- Folder entities appear in the sidebar tree as grouping nodes
-- but have no page content — they exist purely for hierarchy.
ALTER TABLE entities ADD COLUMN is_folder BOOLEAN NOT NULL DEFAULT FALSE AFTER sort_order;
