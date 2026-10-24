-- 002_campaign_system_state: per-campaign, per-system JSON state.
--
-- The campaign-wide twin of entity_system_state, for a document that belongs
-- to the campaign rather than to one page (for example the XP a D&D party
-- handles, which the encounter builder's panel measures every fight against).
-- Same halves, same validation in the service, same collation as the core
-- tables it references. Core migrations run before plugin ones, so referencing
-- core tables is safe here.
CREATE TABLE IF NOT EXISTS campaign_system_state (
    campaign_id  CHAR(36)     NOT NULL,
    system_id    VARCHAR(64)  NOT NULL,
    state_key    VARCHAR(64)  NOT NULL,
    gm_data      JSON         NOT NULL,
    public_data  JSON         NOT NULL,
    updated_by   CHAR(36)     NULL,
    updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id, system_id, state_key),
    CONSTRAINT fk_campaign_system_state_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_campaign_system_state_user
        FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
