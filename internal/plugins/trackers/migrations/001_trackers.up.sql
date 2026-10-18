-- 001_trackers: homebrew trackers and clocks on pages.
--
-- A kind is a campaign's own definition (a number, named steps, or a clock),
-- made by its Directors; nothing here knows any game system. A tracker is a
-- kind placed on a page under a name. Its value is never stored: it is the
-- sum of its log lines, so a mistake is struck rather than overwritten and
-- every change keeps its in-game date. Core tables run before plugin ones,
-- and the collation matches them, so the foreign keys are safe.
CREATE TABLE IF NOT EXISTS tracker_kinds (
    id            CHAR(36)     NOT NULL,
    campaign_id   CHAR(36)     NOT NULL,
    name          VARCHAR(80)  NOT NULL,
    shape         VARCHAR(12)  NOT NULL,
    min_value     INT          NULL,
    max_value     INT          NULL,
    steps         TEXT         NOT NULL,
    bands         TEXT         NOT NULL,
    visibility    VARCHAR(12)  NOT NULL DEFAULT 'directors',
    every_type_id INT          NULL,
    version       INT          NOT NULL DEFAULT 1,
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_tracker_kinds_name (campaign_id, name),
    CONSTRAINT fk_tracker_kinds_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- kind_id has no cascade: a kind still placed on a page cannot be deleted,
-- so no page loses a tracker by accident.
CREATE TABLE IF NOT EXISTS trackers (
    id           CHAR(36)    NOT NULL,
    campaign_id  CHAR(36)    NOT NULL,
    entity_id    CHAR(36)    NOT NULL,
    kind_id      CHAR(36)    NOT NULL,
    name         VARCHAR(80) NOT NULL,
    visibility   VARCHAR(12) NOT NULL DEFAULT 'directors',
    sort_order   INT         NOT NULL DEFAULT 0,
    created_at   DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_trackers_page_name (entity_id, name),
    KEY idx_trackers_campaign (campaign_id),
    KEY idx_trackers_kind (kind_id),
    CONSTRAINT fk_trackers_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE,
    CONSTRAINT fk_trackers_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_trackers_kind     FOREIGN KEY (kind_id)     REFERENCES tracker_kinds(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One line per change. op is 'add' or 'set'; struck lines stay for the
-- record and count for nothing. The in-game date is the default calendar's
-- at the time of the change, NULL when the campaign has none.
CREATE TABLE IF NOT EXISTS tracker_entries (
    id          BIGINT       NOT NULL AUTO_INCREMENT,
    tracker_id  CHAR(36)     NOT NULL,
    campaign_id CHAR(36)     NOT NULL,
    op          VARCHAR(8)   NOT NULL,
    amount      INT          NOT NULL,
    note        VARCHAR(200) NOT NULL DEFAULT '',
    source      VARCHAR(12)  NOT NULL DEFAULT 'hand',
    game_year   INT          NULL,
    game_month  INT          NULL,
    game_day    INT          NULL,
    game_label  VARCHAR(80)  NOT NULL DEFAULT '',
    created_by  CHAR(36)     NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    struck_at   DATETIME     NULL,
    struck_by   CHAR(36)     NULL,
    PRIMARY KEY (id),
    KEY idx_tracker_entries_tracker (tracker_id, id),
    CONSTRAINT fk_tracker_entries_tracker FOREIGN KEY (tracker_id) REFERENCES trackers(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
