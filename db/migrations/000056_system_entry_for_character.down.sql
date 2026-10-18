-- Reverse 000056: entries kept to one character fall back to Directors-only
-- before the value leaves the ENUM, then the column goes.
UPDATE campaign_system_entries SET visibility = 'directors' WHERE visibility = 'character';
ALTER TABLE campaign_system_entries
    MODIFY COLUMN visibility ENUM('everyone','directors') NOT NULL DEFAULT 'everyone';
ALTER TABLE campaign_system_entries DROP COLUMN IF EXISTS for_entity_id;
