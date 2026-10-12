-- 027_entity_era_versions (down) — drop every page's era versions. Destroys
-- what was stored in them; each page then shows its usual name, picture,
-- text and fields in every era. Idempotent.
DROP TABLE IF EXISTS entity_era_versions;
