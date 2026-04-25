-- Data-fix migration: ensure game system addons are active and enabled for
-- campaigns that have them selected. Heals state left over from before the
-- self-healing and version-optional changes were deployed.
--
-- Layering note: this migration touches ONLY core tables (addons, campaigns,
-- campaign_addons). Plugin-owned tables are NOT referenced here — core
-- migrations run before plugin migrations, so on a fresh DB those tables
-- don't exist yet. Any heals that span plugin schema live in the owning
-- plugin's migrations/ directory (see ADR-028).

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
