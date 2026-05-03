-- media.content_hash powers per-campaign upload deduplication: when a user
-- uploads bytes whose sha256 already exists for that campaign, we return
-- the existing media file instead of writing a duplicate to disk + DB.
--
-- CHAR(64) is the hex-encoded sha256 length. Nullable because pre-
-- migration rows have no hash yet — startup runs a backfill goroutine
-- that hashes those files from disk and fills the column. New uploads
-- always populate it.
--
-- Index is per-campaign + hash: dedup lookups always scope to the
-- uploader's campaign (cross-campaign dedup is intentionally OFF —
-- keeps each campaign's media space isolated for clean export/cascade).
ALTER TABLE media_files
    ADD COLUMN content_hash CHAR(64) NULL AFTER file_size,
    ADD INDEX idx_media_files_campaign_hash (campaign_id, content_hash);
