-- A guest code can be a look-around pass instead of a way to join: the person
-- who uses it may read the campaign as a visitor for visit_days days, without
-- becoming a member. visit_until is when their visit ends; it is set when the
-- code is used, moved to now when the owner ends it early, and cleared when
-- the owner makes them a player.
-- Additive and idempotent; campaign_guest_codes is a core table.

ALTER TABLE campaign_guest_codes
    ADD COLUMN IF NOT EXISTS visit_days TINYINT UNSIGNED NULL DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS visit_until DATETIME NULL DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_campaign_guest_codes_visitor
    ON campaign_guest_codes (campaign_id, used_by);
