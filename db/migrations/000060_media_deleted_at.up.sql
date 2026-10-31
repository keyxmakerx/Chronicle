-- media_files.deleted_at puts a campaign picture in its campaign's Trash. The
-- row and the file on disk stay, so pages using the picture keep showing it
-- and Restore is one update; the Trash's clean-up removes both for good.
-- Additive and idempotent; media_files is a core table.

ALTER TABLE media_files
    ADD COLUMN IF NOT EXISTS deleted_at DATETIME NULL DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_media_deleted_at ON media_files (deleted_at);
