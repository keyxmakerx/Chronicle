-- 002_kind_owner: a kind another plugin keeps in step with a record of its
-- own. A Homebrew Workshop recipe's tracker or clock part is saved as a kind
-- whose owner names the recipe and the part, so saving the recipe again
-- updates that kind instead of making a second one, and the kind can't be
-- edited around the recipe. NULL is a kind a Director made by hand.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE tracker_kinds
    ADD COLUMN IF NOT EXISTS owner VARCHAR(160) NULL;

ALTER TABLE tracker_kinds
    ADD UNIQUE KEY IF NOT EXISTS uq_tracker_kinds_owner (campaign_id, owner);
