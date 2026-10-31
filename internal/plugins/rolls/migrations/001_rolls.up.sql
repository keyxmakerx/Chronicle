-- 001_rolls: the campaign's roll log and the table's dice look.
--
-- A roll is kept as made: who rolled, the dice as they fell, the total and
-- who may see it (mode). origin says which app decided the numbers; a roll
-- from Foundry keeps Foundry's chat message id in origin_id, and the unique
-- key on it makes a replay of the same message a no-op. roller_id carries no
-- foreign key: it only records who rolled and must not block removing a
-- user, and roller_name keeps the name the roll was made under.
--
-- campaign_roll_settings holds the table's dice look, which a new player
-- starts on until they pick their own.
CREATE TABLE IF NOT EXISTS campaign_rolls (
    id          CHAR(36)     NOT NULL,
    campaign_id CHAR(36)     NOT NULL,
    origin      VARCHAR(10)  NOT NULL DEFAULT 'chronicle',
    origin_id   VARCHAR(80)  NULL,
    source      VARCHAR(20)  NOT NULL DEFAULT 'table',
    roller_id   CHAR(36)     NULL,
    roller_name VARCHAR(100) NOT NULL DEFAULT '',
    label       VARCHAR(160) NOT NULL DEFAULT '',
    formula     VARCHAR(120) NOT NULL DEFAULT '',
    dice        TEXT         NOT NULL,
    modifier    INT          NOT NULL DEFAULT 0,
    total       INT          NOT NULL DEFAULT 0,
    outcome     VARCHAR(160) NOT NULL DEFAULT '',
    why         VARCHAR(300) NOT NULL DEFAULT '',
    from_where  VARCHAR(160) NOT NULL DEFAULT '',
    mode        VARCHAR(6)   NOT NULL DEFAULT 'public',
    created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_campaign_rolls_origin (campaign_id, origin, origin_id),
    KEY idx_campaign_rolls_recent (campaign_id, created_at),
    CONSTRAINT fk_campaign_rolls_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS campaign_roll_settings (
    campaign_id CHAR(36)    NOT NULL,
    look        VARCHAR(8)  NOT NULL DEFAULT 'quick',
    updated_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (campaign_id),
    CONSTRAINT fk_campaign_roll_settings_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
