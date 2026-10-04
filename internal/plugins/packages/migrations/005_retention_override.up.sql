-- 005_retention_override: per-package override of the site-wide old-version
-- rule. NULL means "use the site rule"; a number N means "keep the newest N
-- versions of this package". Nullable so every existing row keeps today's
-- behaviour. Idempotent DDL per migration safety rules.
ALTER TABLE packages
  ADD COLUMN IF NOT EXISTS retention_keep_newest INT NULL;
