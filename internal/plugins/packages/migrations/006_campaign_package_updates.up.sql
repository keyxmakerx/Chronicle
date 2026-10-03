-- 005_campaign_package_updates: per-campaign update choice for a package.
--
-- One row per (campaign, package) records how that campaign follows the
-- package: update_mode (automatic | pinned | approve_first), the version it
-- stays on (version), and a newly installed version waiting for the owner's
-- approval (held_version). A campaign with no row is "automatic", which is
-- exactly how every campaign behaved before this table existed, so no
-- existing campaign needs a row and no data is backfilled here.
--
-- For a package type whose version lives elsewhere (the Foundry module keeps
-- its pin and pin mode in the campaign's settings, which the manifest
-- endpoint serves from) only held_version is meaningful on its rows; the
-- update_mode and version columns stay at their defaults there.
--
-- campaign_id carries the campaigns table's explicit collation so the foreign
-- key is accepted whatever the database default is; package_id inherits the
-- default, matching packages.id the same way package_versions does.
--
-- Idempotent: safe to re-run after a partial failure.
CREATE TABLE IF NOT EXISTS campaign_package_updates (
    campaign_id   CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
    package_id    CHAR(36) NOT NULL,
    update_mode   ENUM('automatic', 'pinned', 'approve_first') NOT NULL DEFAULT 'automatic',
    version       VARCHAR(50) NULL,
    held_version  VARCHAR(50) NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id, package_id),
    KEY idx_cpu_package (package_id),
    CONSTRAINT fk_cpu_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_cpu_package FOREIGN KEY (package_id) REFERENCES packages(id) ON DELETE CASCADE
) ENGINE=InnoDB;
