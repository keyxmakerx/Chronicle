-- 001_entity_system_state: per-entity, per-system JSON state.
--
-- Column types and collation mirror the core tables they reference
-- (entities.id, campaigns.id and users.id are CHAR(36) utf8mb4_unicode_ci) so
-- the foreign keys are accepted. Core migrations run before plugin ones, so
-- referencing core tables is safe here.
--
-- system_id and state_key are validated in the service (^[a-z0-9][a-z0-9_-]{0,63}$),
-- not by the database, so a new system needs no schema change.
--
-- The two JSON halves are split by audience: public_data may be shown to any
-- viewer of the page, gm_data only to DM-team callers. They are separate
-- columns so a partial write can replace one without touching the other.
--
-- campaign_id is stored beside entity_id so every read can be scoped to the
-- caller's campaign; an entity id from another campaign never matches.
CREATE TABLE IF NOT EXISTS entity_system_state (
    entity_id    CHAR(36)     NOT NULL,
    campaign_id  CHAR(36)     NOT NULL,
    system_id    VARCHAR(64)  NOT NULL,
    state_key    VARCHAR(64)  NOT NULL,
    gm_data      JSON         NOT NULL,
    public_data  JSON         NOT NULL,
    updated_by   CHAR(36)     NULL,
    updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (entity_id, system_id, state_key),
    INDEX idx_entity_system_state_campaign (campaign_id),
    CONSTRAINT fk_entity_system_state_entity
        FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_system_state_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_system_state_user
        FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
