-- Add owner_user_id to entities so any entity (typically character entities)
-- can be claimed by or assigned to a player. Nullable on purpose: most
-- entities (locations, factions, lore) don't have an owner; only character-
-- shaped entities populate this. Same shape as parent_node_id from
-- migration 000013 — column + index + FK with ON DELETE SET NULL so
-- removing a user orphans their entities rather than cascade-deleting
-- campaign content.
--
-- Indexed by (campaign_id, owner_user_id) because the player landing query
-- ("characters this user owns in this campaign") is the dominant access
-- pattern. Per-campaign scoping in the lookup also keeps the user
-- experience isolated across campaigns even when the same user belongs
-- to several.
--
-- Populated by:
--   - Foundry sync (POST /api/v1/.../entities with owner_user_id, after
--     CH5 lands).
--   - Player claim flow on entity show (after CH3 lands).
--   - Owner assignment UI on entity edit (after CH3 lands).

ALTER TABLE entities
    ADD COLUMN owner_user_id CHAR(36) NULL AFTER created_by;

ALTER TABLE entities
    ADD INDEX idx_entities_campaign_owner (campaign_id, owner_user_id);

ALTER TABLE entities
    ADD CONSTRAINT fk_entities_owner_user
    FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE SET NULL;
