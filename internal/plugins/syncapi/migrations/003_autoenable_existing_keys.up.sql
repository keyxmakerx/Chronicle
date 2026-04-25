-- Auto-enable the sync-api addon for campaigns that have at least one API key
-- but don't have the addon enabled. Ensures the sync API shows as active in
-- the features list for campaigns already using it.
--
-- This heal lives in the syncapi plugin (not in core db/migrations/) because
-- it joins against api_keys, which the syncapi plugin owns. Core migrations
-- run before plugin migrations, so on a fresh DB api_keys does not yet exist
-- when core migrations run. By the time this plugin migration executes,
-- 001_sync_tables has already created api_keys, and core has created
-- campaign_addons / addons.

INSERT INTO campaign_addons (campaign_id, addon_id, enabled, enabled_by)
SELECT DISTINCT ak.campaign_id, a.id, 1, NULL
FROM api_keys ak
JOIN addons a ON a.slug = 'sync-api'
WHERE a.id IS NOT NULL
ON DUPLICATE KEY UPDATE enabled = 1;
