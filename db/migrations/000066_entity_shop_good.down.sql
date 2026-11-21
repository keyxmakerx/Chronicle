-- Reverse 000066: the pages stay, and every shop good becomes an ordinary page.
DROP INDEX IF EXISTS idx_entities_campaign_shop_good ON entities;
ALTER TABLE entities DROP COLUMN IF EXISTS shop_good;
