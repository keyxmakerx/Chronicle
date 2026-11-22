-- A game system can offer to take over a category the campaign already has
-- (Draw Steel's treasure fields for Items) with a one-time switch the owner
-- accepts. Two tables hold it.
--
-- entity_type_field_switches is one row per category the owner has answered:
-- 'switched' (the system's fields are on its pages), 'dismissed' ("Not now")
-- or 'back' (switched, then switched back). No row means the owner has not
-- answered, so the offer shows. old_fields is the category's field list from
-- before the switch, which switching back restores; old_preset_category is
-- its preset category from before, restored the same way.
--
-- entity_set_aside_values holds, per page, the values the switch took off it
-- (the fields it set aside, and a reused field's value that does not fit the
-- new field). They are shown only to the GM and come back on switching back,
-- so nothing a GM typed is lost. Kept out of entities.fields_data so no
-- egress path or full-map save can show or drop them.
--
-- Both reference only core tables. No data to backfill: no row means the
-- owner has not been asked yet.

CREATE TABLE IF NOT EXISTS entity_type_field_switches (
    entity_type_id      INT          NOT NULL PRIMARY KEY,
    campaign_id         CHAR(36)     NOT NULL,
    system_id           VARCHAR(100) NOT NULL,
    preset_slug         VARCHAR(100) NOT NULL,
    state               VARCHAR(20)  NOT NULL,
    old_fields          JSON         NULL,
    old_preset_category VARCHAR(50)  NULL,
    updated_by          CHAR(36)     NOT NULL DEFAULT '',
    updated_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    KEY idx_entity_type_field_switches_campaign (campaign_id),
    CONSTRAINT fk_entity_type_field_switches_type
        FOREIGN KEY (entity_type_id) REFERENCES entity_types(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS entity_set_aside_values (
    entity_id      CHAR(36) NOT NULL PRIMARY KEY,
    entity_type_id INT      NOT NULL,
    field_values   JSON     NOT NULL,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    KEY idx_entity_set_aside_values_type (entity_type_id),
    CONSTRAINT fk_entity_set_aside_values_entity
        FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
