-- 004_last_error: durable per-package failure record. Written on install /
-- update-check / post-install-verify failures, cleared on verified success.
-- Surfaced as a badge + banner on /admin/packages so background failures
-- (auto-update worker, load-after-install) survive restarts and are visible
-- without reading server logs. Idempotent DDL per migration safety rules.
ALTER TABLE packages
  ADD COLUMN IF NOT EXISTS last_error TEXT NULL,
  ADD COLUMN IF NOT EXISTS last_error_at DATETIME NULL;
