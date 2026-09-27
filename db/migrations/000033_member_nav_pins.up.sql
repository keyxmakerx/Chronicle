-- A member's own pinned sidebar rows in this campaign: a JSON array of row
-- keys ("app:<slug>", "cat:<id>", "link:<id>") in the order pinned; NULL for
-- none. It lives on the member row so a member's pins are theirs alone and go
-- with their membership. The owner pins for everyone in sidebar_config.
-- Core table, core migration: no plugin tables referenced.

ALTER TABLE campaign_members ADD COLUMN IF NOT EXISTS nav_pins JSON NULL AFTER character_entity_id;
