-- 007_version_digest: the SHA-256 of a version's first download. A later
-- download of the same version must match it, so a moved tag or replaced
-- release asset can't change what an installed version contains. NULL until
-- the version is first downloaded. Idempotent DDL per migration safety rules.
ALTER TABLE package_versions
  ADD COLUMN IF NOT EXISTS content_sha256 CHAR(64) NULL;
