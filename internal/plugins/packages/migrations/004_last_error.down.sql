ALTER TABLE packages
  DROP COLUMN IF EXISTS last_error,
  DROP COLUMN IF EXISTS last_error_at;
