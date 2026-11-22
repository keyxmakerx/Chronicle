-- 002_deck_moments: cards that wait for their moment.
--
-- A card's tests live in deck_cards.when_json (a list of at most four). has_when
-- says the list is not empty, so the question the server asks after every page
-- save ("does this campaign watch anything?") is one indexed lookup.
--
-- said_at is the Director saying so for a card whose test is "the Director says
-- so"; it is cleared when a copy of the card leaves the pile. fits_since is when
-- the card's tests started to hold, so a moment is raised on the rising edge
-- and not again while the tests keep holding.
--
-- A moment is one waiting notice for one deck: the card the deck picked blind
-- among the cards that fit, remembered and not dealt. open_deck_id is the deck
-- id while the moment is open (waiting or invited) and NULL once it closes, so
-- the unique key allows one open moment per deck and any number of closed ones.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE deck_cards ADD COLUMN IF NOT EXISTS has_when   TINYINT(1) NOT NULL DEFAULT 0;
ALTER TABLE deck_cards ADD COLUMN IF NOT EXISTS said_at    DATETIME   NULL;
ALTER TABLE deck_cards ADD COLUMN IF NOT EXISTS fits_since DATETIME   NULL;
ALTER TABLE deck_cards ADD INDEX IF NOT EXISTS idx_deck_cards_when (campaign_id, has_when);

CREATE TABLE IF NOT EXISTS deck_moments (
    id                CHAR(36)     NOT NULL,
    campaign_id       CHAR(36)     NOT NULL,
    deck_id           CHAR(36)     NOT NULL,
    card_id           CHAR(36)     NOT NULL,
    subject_entity_id CHAR(36)     NULL,
    fit_count         INT          NOT NULL DEFAULT 0,
    reading_text      VARCHAR(500) NOT NULL DEFAULT '',
    reading_source    VARCHAR(32)  NOT NULL DEFAULT '',
    read_at           DATETIME     NULL,
    state             VARCHAR(10)  NOT NULL DEFAULT 'waiting',
    quiet             TINYINT(1)   NOT NULL DEFAULT 0,
    invited_user_id   CHAR(36)     NULL,
    open_deck_id      CHAR(36)     NULL,
    created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    closed_at         DATETIME     NULL,
    closed_by         VARCHAR(36)  NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_deck_moments_open (open_deck_id),
    KEY idx_deck_moments_deck (deck_id, closed_at),
    KEY idx_deck_moments_campaign (campaign_id, state),
    CONSTRAINT fk_deck_moments_campaign FOREIGN KEY (campaign_id)       REFERENCES campaigns(id)  ON DELETE CASCADE,
    CONSTRAINT fk_deck_moments_deck     FOREIGN KEY (deck_id)           REFERENCES decks(id)      ON DELETE CASCADE,
    CONSTRAINT fk_deck_moments_card     FOREIGN KEY (card_id)           REFERENCES deck_cards(id) ON DELETE CASCADE,
    CONSTRAINT fk_deck_moments_subject  FOREIGN KEY (subject_entity_id) REFERENCES entities(id)   ON DELETE SET NULL,
    CONSTRAINT fk_deck_moments_invited  FOREIGN KEY (invited_user_id)   REFERENCES users(id)      ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
