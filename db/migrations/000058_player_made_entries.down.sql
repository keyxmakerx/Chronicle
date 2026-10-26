-- Reverse 000058: drop the permissions, then the draft columns. A draft
-- left behind becomes an ordinary entry kept from players.
DROP TABLE IF EXISTS campaign_entry_grants;
UPDATE campaign_system_entries SET visibility = 'directors' WHERE status <> 'accepted' AND visibility = 'everyone';
ALTER TABLE campaign_system_entries DROP COLUMN IF EXISTS review_note;
ALTER TABLE campaign_system_entries DROP COLUMN IF EXISTS status;
