-- Reverse 000047: drop the account-deleted time.
ALTER TABLE users DROP COLUMN IF EXISTS deleted_at;
