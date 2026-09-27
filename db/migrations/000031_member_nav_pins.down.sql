-- Reverse 000031: drop the member's own sidebar pins.
ALTER TABLE campaign_members DROP COLUMN IF EXISTS nav_pins;
