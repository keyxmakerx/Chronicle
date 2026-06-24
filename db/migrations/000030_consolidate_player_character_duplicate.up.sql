-- One-time, generic consolidation of a duplicate "Player Characters" sub-category.
--
-- WHY: when the Player-Character-Claiming addon was enabled BEFORE a game system
-- (or before this sub-category design existed), a campaign ended up with TWO
-- claimable character sub-categories under "Characters":
--   * a generic addon-made type (preset_category 'player_character') that holds
--     the actual character entities but renders the old generic layout, and
--   * the system's own character type (preset_category 'character', e.g. Draw
--     Steel "Heroes" / slug 'drawsteel-character') that owns the real sheet
--     renderer but is empty.
-- The intended shape is ONE sub-category: the system's character type, holding
-- the entities, rendered by the system's widget. EnsurePlayerCharacterType keeps
-- that shape going forward but is deliberately non-destructive (it never moves or
-- deletes), so a PRE-EXISTING duplicate has to be reconciled once, here.
--
-- WHAT: in every campaign that has EXACTLY ONE generic 'player_character' type
-- AND EXACTLY ONE eligible system character type (preset_category 'character',
-- not the default top-level "Characters" parent), move the generic type's
-- entities onto the system type, then delete the now-empty generic type.
--
-- SAFETY / SCOPE:
--   * Modular — keys off preset_category only; NO system-specific strings. Any
--     pack (5e, etc.) with the same duplicate self-heals identically.
--   * Unambiguous-only — the COUNT(*) = 1 guards mean a campaign with zero, or
--     more than one, of either side is left untouched (a system-less campaign
--     keeps its legitimate generic "Player Characters"; an ambiguous campaign is
--     left for a human). Surfacing those is a follow-up (human-readable notice).
--   * Non-destructive to data — entities are MOVED (only entity_type_id changes;
--     campaign_id + slug are preserved, so uq_entities_campaign_slug can't trip,
--     and player claims in campaign_members.character_entity_id reference the
--     entity id, so they follow the move). The generic type is deleted only
--     AFTER it is empty (NOT EXISTS guard), so its ON DELETE CASCADE to entities
--     removes zero rows. Its discarded generic layout / templates / prompts /
--     sidebar nodes cascade away by design.
--   * Idempotent — once the generic type is gone the source side matches nothing,
--     so a re-run (or a fresh DB) is a no-op.
--   * Core-only — touches just entities + entity_types (both core tables).
--
-- Field VALUES are not fabricated: a moved entity keeps its existing fields_data;
-- empty system fields stay empty until re-entered or Foundry-synced.

-- Step 1 — move entities from the generic stray onto the campaign's single
-- system character type (only in campaigns where both sides are unambiguous).
UPDATE entities AS e
JOIN entity_types AS src
  ON src.id = e.entity_type_id
 AND src.preset_category = 'player_character'
JOIN (
    SELECT campaign_id
    FROM entity_types
    WHERE preset_category = 'player_character'
    GROUP BY campaign_id
    HAVING COUNT(*) = 1
) AS uniq_src ON uniq_src.campaign_id = src.campaign_id
JOIN (
    SELECT campaign_id, MIN(id) AS target_id
    FROM entity_types
    WHERE preset_category = 'character'
      AND slug <> 'character'
      AND is_default = FALSE
    GROUP BY campaign_id
    HAVING COUNT(*) = 1
) AS uniq_tgt ON uniq_tgt.campaign_id = src.campaign_id
SET e.entity_type_id = uniq_tgt.target_id;

-- Step 2 — delete the now-empty generic stray in those same campaigns.
--   * The eligible-campaign set is wrapped in an extra derived table
--     (eligible_campaigns) so it is MATERIALIZED. MariaDB/MySQL refuse to delete
--     from a table that a merged (non-materialized) subquery of the same
--     statement also reads (ER_UPDATE_TABLE_USED / errno 1093); the GROUP BY in
--     the inner selects plus the outer wrapper guarantee materialization, so the
--     entity_types self-reference is safe here.
--   * The NOT EXISTS guard (over entities, a different table) ensures a stray
--     that still holds entities — e.g. a campaign whose target was ambiguous, so
--     Step 1 skipped the move — is never deleted. Deleting an empty type then
--     CASCADEs to zero entities; its discarded layout/templates/prompts/sidebar
--     nodes cascade away by design.
DELETE FROM entity_types
WHERE preset_category = 'player_character'
  AND NOT EXISTS (
      SELECT 1 FROM entities AS e WHERE e.entity_type_id = entity_types.id
  )
  AND campaign_id IN (
      SELECT campaign_id FROM (
          SELECT uniq_src.campaign_id
          FROM (
              SELECT campaign_id
              FROM entity_types
              WHERE preset_category = 'player_character'
              GROUP BY campaign_id
              HAVING COUNT(*) = 1
          ) AS uniq_src
          JOIN (
              SELECT campaign_id
              FROM entity_types
              WHERE preset_category = 'character'
                AND slug <> 'character'
                AND is_default = FALSE
              GROUP BY campaign_id
              HAVING COUNT(*) = 1
          ) AS uniq_tgt ON uniq_tgt.campaign_id = uniq_src.campaign_id
      ) AS eligible_campaigns
  );
