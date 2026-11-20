-- 002_practice_heroes: a player's practice heroes, kept apart from entities.
--
-- A practice hero is only a saved hero-creator result: a name and the picks
-- made in each step. It is deliberately not an entities row, so nothing that
-- lists heroes (the party, the DM Screen, game nights, the AI's party, the
-- Foundry sync) can reach one by construction.
--
-- picks_json holds the creator's {fieldKey: {name, options}} map exactly as
-- validated against the plan; derived fields are never stored. level is only
-- set when a copied hero had a readable level. ask_status is '' (not asked),
-- 'waiting' (the maker asked to play it) or 'declined' (a Director said not
-- now, with ask_note); the service checks the words, not an ENUM.
--
-- user_id has no foreign key on purpose, like campaign_choice_requests: the
-- leave, removal and account-deletion hooks delete a person's rows, and a row
-- must never block removing a user. The campaign cascade clears the rest.
CREATE TABLE IF NOT EXISTS practice_heroes (
    id               CHAR(36)     NOT NULL,
    campaign_id      CHAR(36)     NOT NULL,
    user_id          CHAR(36)     NOT NULL,
    name             VARCHAR(200) NOT NULL,
    picks_json       LONGTEXT     NOT NULL,
    level            INT          NULL,
    copied_from_name VARCHAR(200) NULL,
    ask_status       VARCHAR(16)  NOT NULL DEFAULT '',
    ask_note         VARCHAR(500) NULL,
    asked_at         DATETIME(3)  NULL,
    created_at       DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at       DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_practice_heroes_user (campaign_id, user_id),
    KEY idx_practice_heroes_ask (campaign_id, ask_status),
    CONSTRAINT fk_practice_heroes_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
