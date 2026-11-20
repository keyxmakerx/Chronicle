-- Reverse 000064: drop the notes, old names and left-out lists, then the tie
-- columns. Every campaign entry becomes untied again; its text stays.
DROP TABLE IF EXISTS campaign_system_entry_notes;
DROP TABLE IF EXISTS campaign_system_entry_names;
DROP TABLE IF EXISTS campaign_system_left_out;
DROP INDEX IF EXISTS uq_campaign_system_entries_stands_for ON campaign_system_entries;
ALTER TABLE campaign_system_entries DROP COLUMN IF EXISTS stands_for_seen;
ALTER TABLE campaign_system_entries DROP COLUMN IF EXISTS stands_for;
