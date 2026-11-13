-- 001_design_library: each campaign's saved designs and who they are shared with.
--
-- A design is data only: a list of choices from known parts, validated by the
-- feature that owns its kind (room-theme, crafting-circle, ...). Copies are
-- independent rows, so nothing here links a copy back to its original.
--
-- maker_user_id carries no foreign key on purpose: a design a campaign took
-- must outlive the account that made it, and maker_name keeps the name shown
-- then. design_share_owners does reference users, because a named owner who
-- deletes their account must stop being a recipient.
--
-- Core migrations run before plugin ones, so the references to campaigns and
-- users are safe. Collations match those core tables.
CREATE TABLE IF NOT EXISTS design_library (
    id            CHAR(36)     NOT NULL,
    campaign_id   CHAR(36)     NOT NULL,
    kind          VARCHAR(40)  NOT NULL,
    system_id     VARCHAR(255) NOT NULL DEFAULT '',
    name          VARCHAR(60)  NOT NULL,
    note          VARCHAR(140) NOT NULL DEFAULT '',
    maker_user_id CHAR(36)     NULL,
    maker_name    VARCHAR(100) NOT NULL DEFAULT '',
    data          LONGTEXT     NOT NULL,
    origin        VARCHAR(10)  NOT NULL DEFAULT 'made',
    origin_ref    VARCHAR(255) NULL,
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    INDEX idx_design_library_campaign (campaign_id, kind),
    CONSTRAINT fk_design_library_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One row per shared design. link_token is NULL while the link has never been
-- on; link_on says whether it works now. Turning the link on mints a new token,
-- so an old copied link never comes back to life.
CREATE TABLE IF NOT EXISTS design_shares (
    design_id  CHAR(36)    NOT NULL,
    link_token VARCHAR(32) NULL,
    link_on    BOOLEAN     NOT NULL DEFAULT FALSE,
    PRIMARY KEY (design_id),
    UNIQUE KEY uq_design_shares_token (link_token),
    CONSTRAINT fk_design_shares_design
        FOREIGN KEY (design_id) REFERENCES design_library(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS design_share_owners (
    design_id CHAR(36) NOT NULL,
    user_id   CHAR(36) NOT NULL,
    added_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (design_id, user_id),
    INDEX idx_design_share_owners_user (user_id),
    CONSTRAINT fk_design_share_owners_design
        FOREIGN KEY (design_id) REFERENCES design_library(id) ON DELETE CASCADE,
    CONSTRAINT fk_design_share_owners_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
