-- Reverse 000061. Visitors lose their access; nobody's membership changes.
DROP INDEX IF EXISTS idx_campaign_guest_codes_visitor ON campaign_guest_codes;
ALTER TABLE campaign_guest_codes DROP COLUMN IF EXISTS visit_until;
ALTER TABLE campaign_guest_codes DROP COLUMN IF EXISTS visit_days;
