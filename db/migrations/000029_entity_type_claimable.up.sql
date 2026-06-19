-- Add claimable to entity_types for Player Character Claiming (PC-CLAIM-2).
-- Three-valued on purpose so an Owner's explicit choice is distinguishable
-- from "never configured":
--   NULL  = unset → fall back to the legacy heuristic in isClaimableType
--           (preset_category "character", or a slug ending in "-character").
--   TRUE  = the Owner explicitly allows players to claim this type.
--   FALSE = the Owner explicitly forbids claiming this type (overrides the
--           heuristic).
--
-- Placed AFTER parent_type_id to sit with the other type-shape columns.
-- Core table, core migration — no FK, no plugin tables referenced.

ALTER TABLE entity_types ADD COLUMN claimable BOOLEAN NULL AFTER parent_type_id;
