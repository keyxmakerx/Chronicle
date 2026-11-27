-- Pins put a few chosen rows first in a list, in the order they were pinned;
-- everything else keeps its list's own order. The Director team pins.
--
-- campaign_choice_pins holds the pinned entries of one hero pick list (one
-- campaign, system and field, the Ancestry step for example). entry_key is
-- the entry's source and key, "package:<slug>" or "campaign:<entry id>", so a
-- package slug and a campaign entry id can never name each other. position
-- orders the pins; a pin that no longer names an entry is skipped on read.
--
-- sidebar_pins holds the pages pinned first among their siblings in the
-- sidebar. A page deleted for good takes its pin with it.
--
-- updated_by names a user without a foreign key: a pin outlives the account
-- that set it. Only core tables are referenced. No data to backfill: no row
-- means nothing is pinned.

CREATE TABLE IF NOT EXISTS campaign_choice_pins (
    campaign_id CHAR(36)     NOT NULL,
    system_id   VARCHAR(64)  NOT NULL,
    field_key   VARCHAR(64)  NOT NULL,
    entry_key   VARCHAR(160) NOT NULL,
    position    INT          NOT NULL DEFAULT 0,
    updated_by  CHAR(36)     NOT NULL DEFAULT '',
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (campaign_id, system_id, field_key, entry_key),
    CONSTRAINT fk_campaign_choice_pins_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sidebar_pins (
    campaign_id CHAR(36)    NOT NULL,
    entity_id   CHAR(36)    NOT NULL,
    position    INT         NOT NULL DEFAULT 0,
    updated_by  CHAR(36)    NOT NULL DEFAULT '',
    updated_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (campaign_id, entity_id),
    CONSTRAINT fk_sidebar_pins_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_sidebar_pins_entity
        FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
