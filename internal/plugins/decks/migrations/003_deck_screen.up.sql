-- 003_deck_screen: what the DM Screen's Card decks section remembers, and which
-- dealt cards a player has already been shown.
--
-- deck_screen_picks is one Director's chosen decks for the section, in the order
-- they chose, per campaign. A pick goes with its deck (and with the campaign or
-- the user), so a deleted deck never leaves a dangling pick.
--
-- deck_fresh_seen is a copy a player's overlay has already popped up for, so a
-- second device or a reload does not pop it again. copy_id carries no foreign
-- key: a copy that returns to the pile is given a new id in place, which a
-- foreign key would refuse. Rows are short-lived (the pop-up window is minutes)
-- and are pruned when a new one is written.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS deck_screen_picks (
    campaign_id CHAR(36)          NOT NULL,
    user_id     CHAR(36)          NOT NULL,
    deck_id     CHAR(36)          NOT NULL,
    sort_order  SMALLINT UNSIGNED NOT NULL DEFAULT 0,
    PRIMARY KEY (campaign_id, user_id, deck_id),
    KEY idx_deck_screen_picks_order (campaign_id, user_id, sort_order),
    CONSTRAINT fk_deck_screen_picks_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_deck_screen_picks_user     FOREIGN KEY (user_id)     REFERENCES users(id)     ON DELETE CASCADE,
    CONSTRAINT fk_deck_screen_picks_deck     FOREIGN KEY (deck_id)     REFERENCES decks(id)     ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS deck_fresh_seen (
    copy_id     CHAR(36) NOT NULL,
    user_id     CHAR(36) NOT NULL,
    campaign_id CHAR(36) NOT NULL,
    seen_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (copy_id, user_id),
    KEY idx_deck_fresh_seen_at (seen_at),
    CONSTRAINT fk_deck_fresh_seen_user     FOREIGN KEY (user_id)     REFERENCES users(id)     ON DELETE CASCADE,
    CONSTRAINT fk_deck_fresh_seen_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
