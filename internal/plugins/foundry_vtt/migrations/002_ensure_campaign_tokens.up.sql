-- foundry_vtt migration 002: establish the post-consolidation schema on
-- databases that never had the pre-consolidation one.
--
-- WHY THIS EXISTS (C-SWEEP-R4 / data/fvtt-fresh-db-rename).
--
-- Migration 001 is an UPGRADE-PATH migration: it RENAMEs a table that the
-- since-deleted `foundry_modules` plugin used to create, and DROPs a catalog
-- table that only ever existed on that same upgrade path. On a brand-new
-- self-hosted install neither table has ever existed, so 001's very first
-- statement fails with `Error 1146: Table ... doesn't exist`, the plugin is
-- marked DEGRADED, and — because the runner returns on the first failed
-- migration — no later migration could ever repair it. Every fresh install
-- shipped with the Foundry integration permanently dead.
--
-- 001 cannot be edited (migrations are append-only; ADR-044/045), so the
-- repair is split in two:
--
--   * `foundry_vtt.ReconcileConsolidationState` (Go, runs before the plugin
--     migration runner) records 001 as applied on any database where its
--     RENAME can never succeed, so the runner reaches THIS migration.
--   * This migration then establishes the post-consolidation shape with
--     idempotent DDL, so all three histories converge on the same schema:
--       - fresh install        → CREATE runs, DROP is a no-op
--       - completed upgrade    → both are no-ops (001 already did the work)
--       - upgrade that crashed → whichever half 001 did not reach
--
-- Every statement here is idempotent by construction, so re-running it after
-- a partial failure is safe.

-- The post-rename shape of the per-campaign token table, byte-for-byte what
-- `foundry_modules`' migration 001 created and 001's RENAME carried over.
--
-- The FK constraint deliberately keeps the legacy `fk_fmct_campaign` name
-- rather than a foundry_vtt-namespaced one: MariaDB's RENAME TABLE preserves
-- constraint names, so an upgraded database already calls it that. Matching it
-- here is what makes the fresh and upgraded schemas IDENTICAL — a future
-- migration that touches the constraint by name must not have to branch on
-- which history the database took.
CREATE TABLE IF NOT EXISTS foundry_vtt_campaign_tokens (
    campaign_id    CHAR(36)  NOT NULL,
    token_version  INT       NOT NULL DEFAULT 1,
    rotated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id),
    CONSTRAINT fk_fmct_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- The other half of 001's intent. On a fresh install this table never existed;
-- on a completed upgrade 001 already dropped it; on an upgrade that crashed
-- between 001's two statements it is still here and still needs to go. The
-- empty-row precondition is enforced by foundry_vtt.PreMigrationCheck, which
-- runs before the migration runner and refuses to proceed if it has rows.
DROP TABLE IF EXISTS foundry_module_versions;
