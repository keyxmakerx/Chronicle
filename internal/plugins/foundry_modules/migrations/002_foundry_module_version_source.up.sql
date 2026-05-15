-- Hybrid source model for the Foundry module catalog: rows can now
-- originate from a GitHub release auto-fetch (default) or a manual
-- admin upload. The pre-existing rows are all manual uploads, so
-- DEFAULT 'manual_upload' is the correct backfill.
--
-- github_release_id is BIGINT because GitHub's API release IDs are
-- 64-bit integers. The UNIQUE constraint is what makes the poller
-- idempotent — a second poll of the same release ID hits the unique
-- key, the INSERT errors out with ErrVersionExists, and the row stays
-- single.
--
-- uploaded_by_user_id is relaxed to NULL so the poller can insert
-- without a synthetic system user. The FK constraint stays — it just
-- doesn't fire on NULL, which is allowed.
ALTER TABLE foundry_module_versions
    MODIFY COLUMN uploaded_by_user_id CHAR(36) NULL,
    ADD COLUMN source ENUM('manual_upload', 'github_release') NOT NULL DEFAULT 'manual_upload',
    ADD COLUMN github_release_tag VARCHAR(50) NULL,
    ADD COLUMN github_release_id BIGINT NULL,
    ADD UNIQUE KEY uk_github_release (github_release_id);
