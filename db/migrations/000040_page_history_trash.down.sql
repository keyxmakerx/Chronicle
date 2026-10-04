DROP TABLE IF EXISTS entity_versions;
DROP INDEX IF EXISTS idx_entities_trash ON entities;
ALTER TABLE entities
    DROP COLUMN IF EXISTS entry_rev,
    DROP COLUMN IF EXISTS trash_root_id,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at;
