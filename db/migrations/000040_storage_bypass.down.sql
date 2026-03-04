ALTER TABLE user_storage_limits
  DROP COLUMN bypass_max_upload,
  DROP COLUMN bypass_expires_at,
  DROP COLUMN bypass_reason,
  DROP COLUMN bypass_granted_by;

ALTER TABLE campaign_storage_limits
  DROP COLUMN bypass_max_storage,
  DROP COLUMN bypass_max_files,
  DROP COLUMN bypass_expires_at,
  DROP COLUMN bypass_reason,
  DROP COLUMN bypass_granted_by;
