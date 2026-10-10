-- Reverse 000052: drop guest codes and the guest marker on accounts.
DROP TABLE IF EXISTS campaign_guest_codes;
DROP INDEX IF EXISTS idx_users_guest_campaign ON users;
ALTER TABLE users DROP COLUMN IF EXISTS guest_campaign_id;
