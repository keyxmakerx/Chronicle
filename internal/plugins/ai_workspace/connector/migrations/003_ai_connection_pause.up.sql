-- 003_ai_connection_pause: the owner may pause a key and resume it.
--
-- A paused key is refused until it is resumed. It is not revoked, so the AI
-- app keeps the same key and nothing has to be pasted again.
ALTER TABLE ai_connections ADD COLUMN IF NOT EXISTS paused_at DATETIME NULL;
