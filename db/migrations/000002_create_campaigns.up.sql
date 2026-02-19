-- Migration: 000002_create_campaigns
-- Description: Creates campaigns, campaign_members, and ownership_transfers
--              tables for worldbuilding containers and role-based membership.
-- Related: ADR-001 (three-tier architecture), campaigns plugin

CREATE TABLE IF NOT EXISTS campaigns (
    id          CHAR(36)     NOT NULL,
    name        VARCHAR(200) NOT NULL,
    slug        VARCHAR(200) NOT NULL,
    description TEXT         DEFAULT NULL,

    -- JSON blob for campaign-level settings: enabled modules, theme, etc.
    settings    JSON         NOT NULL DEFAULT ('{}'),

    -- The user who created (and initially owns) this campaign.
    created_by  CHAR(36)     NOT NULL,

    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    UNIQUE KEY idx_campaigns_slug (slug),
    INDEX idx_campaigns_created_by (created_by),
    CONSTRAINT fk_campaigns_created_by FOREIGN KEY (created_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- campaign_members tracks which users have access to which campaigns and
-- with what role. The owner is always a member with role='owner'. Users can
-- be members of many campaigns with different roles in each.
CREATE TABLE IF NOT EXISTS campaign_members (
    campaign_id CHAR(36)     NOT NULL,
    user_id     CHAR(36)     NOT NULL,

    -- Role within this campaign: 'owner', 'scribe', 'player'.
    -- Owner: full control. Scribe: create/edit content. Player: read-only.
    role        VARCHAR(20)  NOT NULL DEFAULT 'player',

    joined_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (campaign_id, user_id),
    INDEX idx_campaign_members_user (user_id),
    CONSTRAINT fk_cm_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_cm_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT chk_cm_role CHECK (role IN ('owner', 'scribe', 'player'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ownership_transfers tracks pending ownership transfer requests.
-- Only one pending transfer per campaign at a time (UNIQUE on campaign_id).
CREATE TABLE IF NOT EXISTS ownership_transfers (
    id            CHAR(36)     NOT NULL,
    campaign_id   CHAR(36)     NOT NULL,
    from_user_id  CHAR(36)     NOT NULL,
    to_user_id    CHAR(36)     NOT NULL,

    -- Random hex token for email verification link.
    token         VARCHAR(128) NOT NULL,

    expires_at    DATETIME     NOT NULL,
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    UNIQUE KEY idx_ot_campaign (campaign_id),
    UNIQUE KEY idx_ot_token (token),
    CONSTRAINT fk_ot_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_ot_from FOREIGN KEY (from_user_id) REFERENCES users(id),
    CONSTRAINT fk_ot_to FOREIGN KEY (to_user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
