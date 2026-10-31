-- Reverse 000060. Pictures still in a campaign Trash come back into use, so
-- rolling back never deletes a file.
DROP INDEX IF EXISTS idx_media_deleted_at ON media_files;
ALTER TABLE media_files DROP COLUMN IF EXISTS deleted_at;
