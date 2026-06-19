-- Reverse 000029: drop the claimable column. No FK or index to remove first.
ALTER TABLE entity_types DROP COLUMN claimable;
