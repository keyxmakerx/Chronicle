-- A site-wide Trash with Undo: a deleted campaign and an admin's file clean-up
-- wait here before anything is removed for good.
--
-- campaigns: deleted_at marks a campaign as in the Trash. Every read that
-- opens a campaign treats a marked row as missing, so nobody can reach it, and
-- its slug stays taken so Undo can never collide with a new campaign.
-- deleted_by_name is a copy of the person's name: the Trash must still say who
-- deleted it after that account is gone, so deleted_by is deliberately not a
-- foreign key. purge_started_at is set by the one actor that begins the final
-- delete; once set, Undo is refused, and a purge that stopped halfway is
-- finished by the next run instead of being left as a half-deleted campaign.
--
-- media_files: trash_batch_id puts a file in a clean-up batch. The file stays
-- on disk and in this table, so Undo is one update and storage still counts it.
--
-- trash_batches: one row per file clean-up. state moves trashed -> restoring
-- (Undo claimed it) or trashed -> purging (the final delete claimed it); each
-- claim is a single conditional update, so exactly one of the two can win.
-- Core tables only (campaigns, media_files); core migration.

ALTER TABLE campaigns
    ADD COLUMN IF NOT EXISTS deleted_at       DATETIME     NULL DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS deleted_by       CHAR(36)     NULL DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS deleted_by_name  VARCHAR(200) NULL DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS purge_started_at DATETIME     NULL DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_campaigns_deleted_at ON campaigns (deleted_at);

ALTER TABLE media_files
    ADD COLUMN IF NOT EXISTS trash_batch_id CHAR(36) NULL DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_media_trash_batch ON media_files (trash_batch_id);

CREATE TABLE IF NOT EXISTS trash_batches (
    id              CHAR(36)         NOT NULL PRIMARY KEY,
    kind            VARCHAR(32)      NOT NULL,
    label           VARCHAR(255)     NOT NULL DEFAULT '',
    item_count      INT UNSIGNED     NOT NULL DEFAULT 0,
    byte_count      BIGINT UNSIGNED  NOT NULL DEFAULT 0,
    state           VARCHAR(16)      NOT NULL DEFAULT 'trashed',
    deleted_by      CHAR(36)         NULL DEFAULT NULL,
    deleted_by_name VARCHAR(200)     NULL DEFAULT NULL,
    created_at      DATETIME         NOT NULL,

    INDEX idx_trash_batches_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
