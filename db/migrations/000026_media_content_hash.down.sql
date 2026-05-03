ALTER TABLE media_files
    DROP INDEX idx_media_files_campaign_hash,
    DROP COLUMN content_hash;
