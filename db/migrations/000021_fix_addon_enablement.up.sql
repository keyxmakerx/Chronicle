-- Data-fix migration: ensure game system addons are active and enabled for
-- campaigns that have them selected. Fixes state left over from before the
-- self-healing and version-optional changes were deployed.

-- 1. Activate all system-category addons that packages have loaded.
--    The baseline migration seeds them as 'planned', but RegisterSystemAddon
--    + SeedInstalledAddons should upsert to 'active'. This catches cases
--    where the system failed to load (e.g., missing manifest version).
UPDATE addons SET status = 'active'
WHERE category = 'system'
  AND slug IN ('drawsteel', 'dnd5e', 'pathfinder2e')
  AND status = 'planned';

-- 2. Ensure sync-api addon is active (should already be, defensive).
UPDATE addons SET status = 'active'
WHERE slug = 'sync-api' AND status != 'active';

-- 3. Auto-enable game system addons for campaigns that have a system_id
--    selected in their settings JSON but don't have the addon enabled.
--    This heals campaigns where the system was selected before addon
--    auto-enablement was deployed.
INSERT INTO campaign_addons (campaign_id, addon_id, enabled, enabled_by)
SELECT c.id, a.id, 1, NULL
FROM campaigns c
JOIN addons a ON a.slug = JSON_UNQUOTE(JSON_EXTRACT(c.settings, '$.system_id'))
WHERE c.settings IS NOT NULL
  AND c.settings != ''
  AND c.settings != '{}'
  AND JSON_VALID(c.settings)
  AND JSON_EXTRACT(c.settings, '$.system_id') IS NOT NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(c.settings, '$.system_id')) != ''
  AND a.category = 'system'
ON DUPLICATE KEY UPDATE enabled = 1;

-- 4. Auto-enable sync-api addon for campaigns that have at least one API key
--    but don't have the addon enabled. This ensures the sync API shows as
--    active in the features list for campaigns already using it.
INSERT INTO campaign_addons (campaign_id, addon_id, enabled, enabled_by)
SELECT DISTINCT ak.campaign_id, a.id, 1, NULL
FROM api_keys ak
JOIN addons a ON a.slug = 'sync-api'
WHERE a.id IS NOT NULL
ON DUPLICATE KEY UPDATE enabled = 1;
