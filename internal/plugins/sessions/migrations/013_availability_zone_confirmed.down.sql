-- Revert 013_availability_zone_confirmed. Members who answered the time zone
-- question would be asked again if their zones still differ.
ALTER TABLE member_availability_status DROP COLUMN IF EXISTS zone_confirmed_at;
