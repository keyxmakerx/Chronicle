-- "Since you were last here" on the campaign home: when each member last
-- pressed "Mark as seen" in this campaign. NULL means never, and the block
-- then reads from the day the member joined. It lives on the member row, as
-- nav_pins does, so it is the member's alone and goes with their membership.
-- Only that button writes it: loading the home must not, or the list would
-- empty before it was read.
--
-- The index lets "pages created since" read one campaign's newest pages
-- without scanning all of them (the block filters on campaign_id and
-- created_at, then on what the viewer may open).
-- Core tables, core migration: no plugin tables referenced. Idempotent.

ALTER TABLE campaign_members ADD COLUMN IF NOT EXISTS home_seen_at DATETIME NULL;

CREATE INDEX IF NOT EXISTS idx_entities_campaign_created ON entities (campaign_id, created_at);
