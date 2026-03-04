-- Add temporary bypass columns to storage limit override tables.
-- Bypasses are time-limited overrides that auto-expire, used by admins
-- during bulk imports, campaign migrations, or one-time large uploads.

ALTER TABLE user_storage_limits
  ADD COLUMN bypass_max_upload BIGINT NULL AFTER max_total_storage,
  ADD COLUMN bypass_expires_at TIMESTAMP NULL AFTER bypass_max_upload,
  ADD COLUMN bypass_reason VARCHAR(255) NULL AFTER bypass_expires_at,
  ADD COLUMN bypass_granted_by CHAR(36) NULL AFTER bypass_reason;

ALTER TABLE campaign_storage_limits
  ADD COLUMN bypass_max_storage BIGINT NULL AFTER max_files,
  ADD COLUMN bypass_max_files INT NULL AFTER bypass_max_storage,
  ADD COLUMN bypass_expires_at TIMESTAMP NULL AFTER bypass_max_files,
  ADD COLUMN bypass_reason VARCHAR(255) NULL AFTER bypass_expires_at,
  ADD COLUMN bypass_granted_by CHAR(36) NULL AFTER bypass_reason;
