-- When a night's recap first went from empty to written, for the campaign
-- home's "Since you were last here". updated_at moves on any edit of the
-- night (a rename, a new time), so it cannot say when a recap was posted.
-- NULL for recaps written before this column existed: they are not news.
-- Set only by the recap save, never backfilled (schema-only migration).
--
-- Both indexes serve that block's reads, which filter one campaign by time:
-- recaps posted since, and nights created since.
-- Idempotent: applies on a fresh database and a second time on top of itself.

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS recap_posted_at DATETIME NULL DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_sessions_recap_posted ON sessions (campaign_id, recap_posted_at);
CREATE INDEX IF NOT EXISTS idx_sessions_campaign_created ON sessions (campaign_id, created_at);
