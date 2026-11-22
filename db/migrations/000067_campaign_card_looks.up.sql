-- The border a campaign's owner gives a game system's own card in the hero
-- creator. A package entry (an ancestry, a class, a derived subclass) has no
-- row of its own to write a choice into, so the choice is kept here, one row
-- per (campaign, system, field, entry). No row means "the game system's own
-- look"; a row with an empty border means "no border". A campaign's own
-- entries do not use this table: their choice is written into the entry's own
-- card_border / card_colour properties.
--
-- entry_key is a package entry's slug, or a derived entry's
-- <parent slug>--<name slug> key. Package slugs are not length-limited by the
-- package format, so the service refuses a key longer than the column rather
-- than storing a truncated one that would name another entry.
--
-- colour is '' for "the border's own colour", else #rrggbb in lower case.
-- updated_by names a user without a foreign key: a look outlives the account
-- that chose it. Only campaigns is referenced (core tables, core migration).
-- No data to backfill: no row means "the game system's own look".

CREATE TABLE IF NOT EXISTS campaign_card_looks (
    campaign_id CHAR(36)     NOT NULL,
    system_id   VARCHAR(64)  NOT NULL,
    field_key   VARCHAR(64)  NOT NULL,
    entry_key   VARCHAR(150) NOT NULL,
    border      VARCHAR(40)  NOT NULL DEFAULT '',
    colour      VARCHAR(7)   NOT NULL DEFAULT '',
    updated_by  CHAR(36)     NOT NULL DEFAULT '',
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (campaign_id, system_id, field_key, entry_key),
    CONSTRAINT fk_campaign_card_looks_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
