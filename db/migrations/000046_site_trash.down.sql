-- Reverse 000046. Anything still in the Trash comes back with it: campaigns
-- become reachable again and clean-up files return to the unused-uploads
-- list, so rolling back never deletes data. Empty the Trash first if that is
-- not wanted.
DROP TABLE IF EXISTS trash_batches;
DROP INDEX IF EXISTS idx_media_trash_batch ON media_files;
ALTER TABLE media_files DROP COLUMN IF EXISTS trash_batch_id;
DROP INDEX IF EXISTS idx_campaigns_deleted_at ON campaigns;
ALTER TABLE campaigns
    DROP COLUMN IF EXISTS purge_started_at,
    DROP COLUMN IF EXISTS deleted_by_name,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at;
