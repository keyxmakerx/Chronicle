-- A campaign reading a game system in its own words.
--
-- stands_for ties a campaign entry to the standard entry (same system and
-- field, by slug) it takes the place of. Only a Director's press makes a tie,
-- never a matching name. The unique key keeps one stand-in per standard
-- entry; NULLs repeat freely, so untied entries are unaffected.
-- stands_for_seen is the standard entry as it was when tied or last marked
-- "Seen" (JSON: name, summary, description, properties), so the Director can
-- be told later what the package changed underneath.
--
-- campaign_system_left_out lists standard entries the campaign does not
-- offer, with the Director's optional sentence.
-- campaign_system_entry_names keeps a renamed entry's old names, so heroes
-- (which store the name) and links made earlier still find it; the unique key
-- keeps an old name from being given to a second entry.
-- campaign_system_entry_notes keeps what the players were told about a change.
--
-- created_by is deliberately not a foreign key, as in 000043: a row must
-- outlive the account that wrote it.
-- Core tables, core migration: only campaigns and campaign_system_entries are
-- referenced.

ALTER TABLE campaign_system_entries
    ADD COLUMN IF NOT EXISTS stands_for VARCHAR(64) NULL DEFAULT NULL AFTER review_note;
ALTER TABLE campaign_system_entries
    ADD COLUMN IF NOT EXISTS stands_for_seen MEDIUMTEXT NULL DEFAULT NULL AFTER stands_for;
CREATE UNIQUE INDEX IF NOT EXISTS uq_campaign_system_entries_stands_for
    ON campaign_system_entries (campaign_id, system_id, field_key, stands_for);

CREATE TABLE IF NOT EXISTS campaign_system_left_out (
    campaign_id CHAR(36)     NOT NULL,
    system_id   VARCHAR(64)  NOT NULL,
    field_key   VARCHAR(64)  NOT NULL,
    slug        VARCHAR(64)  NOT NULL,
    sentence    VARCHAR(300) NOT NULL DEFAULT '',
    created_by  CHAR(36)     DEFAULT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (campaign_id, system_id, field_key, slug),
    CONSTRAINT fk_campaign_system_left_out_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS campaign_system_entry_names (
    id          BIGINT       AUTO_INCREMENT PRIMARY KEY,
    campaign_id CHAR(36)     NOT NULL,
    system_id   VARCHAR(64)  NOT NULL,
    field_key   VARCHAR(64)  NOT NULL,
    name        VARCHAR(100) NOT NULL,
    entry_id    BIGINT       NOT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE KEY uq_campaign_system_entry_names (campaign_id, system_id, field_key, name),
    KEY idx_campaign_system_entry_names_entry (entry_id),
    CONSTRAINT fk_campaign_system_entry_names_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_campaign_system_entry_names_entry
        FOREIGN KEY (entry_id) REFERENCES campaign_system_entries(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS campaign_system_entry_notes (
    id          BIGINT       AUTO_INCREMENT PRIMARY KEY,
    campaign_id CHAR(36)     NOT NULL,
    entry_id    BIGINT       NOT NULL,
    line        VARCHAR(300) NOT NULL,
    created_by  CHAR(36)     DEFAULT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,

    KEY idx_campaign_system_entry_notes_entry (campaign_id, entry_id, created_at),
    KEY idx_campaign_system_entry_notes_entry_fk (entry_id),
    CONSTRAINT fk_campaign_system_entry_notes_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_campaign_system_entry_notes_entry
        FOREIGN KEY (entry_id) REFERENCES campaign_system_entries(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
