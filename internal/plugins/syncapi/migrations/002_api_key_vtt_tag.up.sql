-- Add VTT tag to API keys for visual identification of which VTT tool
-- each key is associated with (e.g., "foundry", "custom").
ALTER TABLE api_keys ADD COLUMN vtt_tag VARCHAR(50) DEFAULT NULL AFTER name;
