-- A shop good is a real page that exists only to be sold: a shop's stock
-- must point at a page, and most of what a filled shop sells (rope, lamp oil,
-- a system treasure) has none. The mark keeps such a page out of lists,
-- search, Auto-link and sync until the Director promotes it to a full page.
-- It is deliberately not a visibility: players must still open the page from
-- the shop and from their pack, and a hidden page would drop from the shop.
-- The column is inverted (false = an ordinary page) so a row written without
-- it, or by code that predates it, is an ordinary page.
ALTER TABLE entities
    ADD COLUMN IF NOT EXISTS shop_good BOOLEAN NOT NULL DEFAULT FALSE AFTER is_template;
CREATE INDEX IF NOT EXISTS idx_entities_campaign_shop_good
    ON entities (campaign_id, shop_good);
