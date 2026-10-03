-- The version is a self-reported diagnostic, re-learned on the module's next
-- request, so dropping it loses no source-of-truth data.
ALTER TABLE api_keys DROP COLUMN IF EXISTS module_version;
