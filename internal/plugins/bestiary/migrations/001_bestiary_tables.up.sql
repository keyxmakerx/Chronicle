-- Community Bestiary tables: publications, ratings, favorites, imports, moderation log.
-- Publications are instance-scoped (not campaign-scoped) — visible to all authenticated users.

CREATE TABLE IF NOT EXISTS bestiary_publications (
    id                CHAR(36)     NOT NULL PRIMARY KEY,
    creator_id        CHAR(36)     NOT NULL,
    source_entity_id  CHAR(36)     DEFAULT NULL,
    source_campaign_id CHAR(36)    DEFAULT NULL,
    system_id         VARCHAR(100) NOT NULL DEFAULT 'drawsteel',
    name              VARCHAR(200) NOT NULL,
    slug              VARCHAR(200) NOT NULL,
    description       TEXT         DEFAULT NULL,
    flavor_text       TEXT         DEFAULT NULL,
    artwork_media_id  CHAR(36)     DEFAULT NULL,
    statblock_json    JSON         NOT NULL,
    version           INT          NOT NULL DEFAULT 1,
    tags              JSON         DEFAULT NULL,
    organization      VARCHAR(50)  DEFAULT NULL,
    role              VARCHAR(50)  DEFAULT NULL,
    level             INT          DEFAULT NULL,
    downloads         INT          NOT NULL DEFAULT 0,
    rating_sum        INT          NOT NULL DEFAULT 0,
    rating_count      INT          NOT NULL DEFAULT 0,
    favorites         INT          NOT NULL DEFAULT 0,
    visibility        ENUM('draft','published','unlisted','archived','flagged') NOT NULL DEFAULT 'draft',
    flagged_count     INT          NOT NULL DEFAULT 0,
    reviewed_by       CHAR(36)     DEFAULT NULL,
    reviewed_at       DATETIME     DEFAULT NULL,
    hub_id            VARCHAR(100) DEFAULT NULL,
    hub_synced_at     DATETIME     DEFAULT NULL,
    created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_bp_system_visibility (system_id, visibility),
    INDEX idx_bp_level_org_role (level, organization, role),
    INDEX idx_bp_creator (creator_id),
    UNIQUE INDEX idx_bp_slug (slug),
    FULLTEXT INDEX idx_bp_search (name, description),

    CONSTRAINT fk_bp_creator FOREIGN KEY (creator_id)
        REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS bestiary_ratings (
    id              CHAR(36)  NOT NULL PRIMARY KEY,
    publication_id  CHAR(36)  NOT NULL,
    user_id         CHAR(36)  NOT NULL,
    rating          TINYINT   NOT NULL CHECK (rating BETWEEN 1 AND 5),
    review_text     TEXT      DEFAULT NULL,
    created_at      DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE INDEX idx_br_user_pub (user_id, publication_id),

    CONSTRAINT fk_br_publication FOREIGN KEY (publication_id)
        REFERENCES bestiary_publications(id) ON DELETE CASCADE,
    CONSTRAINT fk_br_user FOREIGN KEY (user_id)
        REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS bestiary_favorites (
    user_id         CHAR(36)  NOT NULL,
    publication_id  CHAR(36)  NOT NULL,
    created_at      DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (user_id, publication_id),

    CONSTRAINT fk_bf_publication FOREIGN KEY (publication_id)
        REFERENCES bestiary_publications(id) ON DELETE CASCADE,
    CONSTRAINT fk_bf_user FOREIGN KEY (user_id)
        REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS bestiary_imports (
    id              CHAR(36)  NOT NULL PRIMARY KEY,
    publication_id  CHAR(36)  NOT NULL,
    user_id         CHAR(36)  NOT NULL,
    campaign_id     CHAR(36)  NOT NULL,
    entity_id       CHAR(36)  DEFAULT NULL,
    imported_at     DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE INDEX idx_bi_pub_campaign (publication_id, campaign_id),

    CONSTRAINT fk_bi_publication FOREIGN KEY (publication_id)
        REFERENCES bestiary_publications(id) ON DELETE CASCADE,
    CONSTRAINT fk_bi_user FOREIGN KEY (user_id)
        REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS bestiary_moderation_log (
    id              CHAR(36)  NOT NULL PRIMARY KEY,
    publication_id  CHAR(36)  NOT NULL,
    moderator_id    CHAR(36)  NOT NULL,
    action          ENUM('approve','flag','unflag','archive','restore') NOT NULL,
    reason          TEXT      DEFAULT NULL,
    created_at      DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_bml_publication (publication_id),

    CONSTRAINT fk_bml_publication FOREIGN KEY (publication_id)
        REFERENCES bestiary_publications(id) ON DELETE CASCADE,
    CONSTRAINT fk_bml_moderator FOREIGN KEY (moderator_id)
        REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
