-- Revert 011_recap_posted_at. The posted times are lost; recaps themselves stay.
DROP INDEX IF EXISTS idx_sessions_campaign_created ON sessions;
DROP INDEX IF EXISTS idx_sessions_recap_posted ON sessions;
ALTER TABLE sessions DROP COLUMN IF EXISTS recap_posted_at;
