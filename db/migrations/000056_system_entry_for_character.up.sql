-- A campaign entry kept to one character: visibility 'character' shows it
-- only to Directors and to whoever plays the page in for_entity_id, so a
-- penciled ancestry can be shown to its one player before the party sees it.
-- for_entity_id is deliberately not a foreign key, like created_by: deleting
-- the character must not delete the entry, and the service treats an entry
-- whose character is gone as Directors-only.
-- Core table, core migration: no plugin tables referenced.

ALTER TABLE campaign_system_entries
    MODIFY COLUMN visibility ENUM('everyone','directors','character') NOT NULL DEFAULT 'everyone';
ALTER TABLE campaign_system_entries
    ADD COLUMN IF NOT EXISTS for_entity_id CHAR(36) NULL DEFAULT NULL AFTER visibility;
