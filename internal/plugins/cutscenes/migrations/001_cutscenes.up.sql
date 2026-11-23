-- 001_cutscenes: cut scenes and their showings.
--
-- A scene's layers are stored as the JSON the service checked and clamped,
-- one column per layer so each can be read and changed on its own. A showing
-- keeps the whole scene as it was shown (scene), so the people it was for
-- can watch it again after the scene is edited or deleted; scene_id is then
-- cleared, not cascaded. shown_by and created_by carry no foreign key: losing
-- an account must not take a campaign's scenes or its showings with it.
--
-- New tables only; core tables run before plugin ones and the collation
-- matches them, so the foreign keys are safe.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS cutscenes (
    id          CHAR(36)     NOT NULL,
    campaign_id CHAR(36)     NOT NULL,
    name        VARCHAR(120) NOT NULL,
    background  TEXT         NOT NULL,
    overlays    TEXT         NOT NULL,
    text        TEXT         NOT NULL,
    ending      TEXT         NOT NULL,
    created_by  CHAR(36)     NOT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_cutscenes_name (campaign_id, name),
    CONSTRAINT fk_cutscenes_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One play of a scene to named people. rewatch_until ends the window in which
-- those people may play it again; stopped_at is when the Director pulled it
-- off live screens, which leaves the window as it is.
CREATE TABLE IF NOT EXISTS cutscene_showings (
    id            CHAR(36)  NOT NULL,
    campaign_id   CHAR(36)  NOT NULL,
    scene_id      CHAR(36)  NULL,
    scene         TEXT      NOT NULL,
    shown_by      CHAR(36)  NOT NULL,
    shown_at      DATETIME  NOT NULL,
    rewatch_until DATETIME  NOT NULL,
    stopped_at    DATETIME  NULL,
    PRIMARY KEY (id),
    KEY idx_cutscene_showings_campaign (campaign_id, shown_at),
    KEY idx_cutscene_showings_scene (scene_id),
    CONSTRAINT fk_cutscene_showings_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_cutscene_showings_scene    FOREIGN KEY (scene_id)    REFERENCES cutscenes(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Who a showing was for. watched_at is the first time that person's screen
-- played it, set once.
CREATE TABLE IF NOT EXISTS cutscene_showing_viewers (
    showing_id CHAR(36) NOT NULL,
    user_id    CHAR(36) NOT NULL,
    watched_at DATETIME NULL,
    PRIMARY KEY (showing_id, user_id),
    KEY idx_cutscene_viewers_user (user_id),
    CONSTRAINT fk_cutscene_viewers_showing FOREIGN KEY (showing_id) REFERENCES cutscene_showings(id) ON DELETE CASCADE,
    CONSTRAINT fk_cutscene_viewers_user    FOREIGN KEY (user_id)    REFERENCES users(id)             ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
