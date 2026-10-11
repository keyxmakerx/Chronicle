-- Reverse 000053: drop the last-seen time.
ALTER TABLE users DROP COLUMN IF EXISTS last_seen_at;
