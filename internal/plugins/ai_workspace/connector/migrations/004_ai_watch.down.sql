DROP TABLE IF EXISTS ai_connection_alerts;
DROP TABLE IF EXISTS ai_server_access;
DROP TABLE IF EXISTS ai_campaign_access;
DROP INDEX IF EXISTS idx_ai_connections_person ON ai_connections;
ALTER TABLE ai_connections DROP COLUMN IF EXISTS seen_apps;
