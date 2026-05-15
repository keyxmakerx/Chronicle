-- C-FMC-5c migration 001: consolidate from foundry_modules to foundry_vtt.
--
-- Renames the per-campaign token table out of foundry_modules' namespace
-- (now deleted) into foundry_vtt's. Drops the foundry_module_versions
-- catalog table — the new architecture uses packages plugin's package_versions
-- table as the single source of version truth.
--
-- IMPORTANT: An empty-row precondition on foundry_module_versions is
-- enforced via a Go-side check (foundry_vtt.PreMigrationCheck) called from
-- cmd/server/main.go BEFORE plugin migrations run. The SQL here assumes
-- foundry_module_versions is empty; the Go pre-check aborts startup with
-- an actionable error message if it isn't.
--
-- Why a Go-side pre-check (vs. an in-SQL stored procedure SIGNAL): golang-
-- migrate splits migrations on ';' before sending to the driver, which
-- breaks the DELIMITER syntax that CREATE PROCEDURE needs. Plain SQL can't
-- emit a SIGNAL outside a procedure. The Go-side check is the cleanest
-- option that produces an actionable error message.

-- Rename per-campaign token table to its final namespace. Existing rows
-- (campaign_id, token_version, rotated_at) are preserved. The schema is
-- unchanged.
RENAME TABLE foundry_module_campaign_tokens TO foundry_vtt_campaign_tokens;

-- Drop the orphaned versions catalog table. Foundry-module versions now
-- come from packages plugin's package_versions table (queried by foundry_vtt
-- via the PackageReader interface). The CASCADE / RESTRICT default isn't
-- relevant — the table has no FK references AFTER the rename above, since
-- the only inbound FK was foundry_module_campaign_tokens → campaigns (not
-- → foundry_module_versions).
DROP TABLE foundry_module_versions;
