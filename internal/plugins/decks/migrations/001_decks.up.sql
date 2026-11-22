-- 001_decks: card decks for tabletop campaigns.
--
-- A deck holds card definitions; a definition with N copies has N rows in
-- deck_copies, one per physical card, and a copy is in exactly one zone.
-- The pile has no stored order: a draw picks at random among the pile's
-- copies when it happens, so there is no order to leak. A copy that returns
-- to the pile is given a new id in the same statement, so an id a player has
-- seen beside a face never comes back on a hidden card. New tables only;
-- core tables run before plugin ones and the collation matches them, so the
-- foreign keys are safe.
--
-- next_card_id and last_card_id are a Director's two nudges to the draw: the
-- card the next draw takes, and the card left out while anything else is in
-- the pile. They carry no foreign key because the repository clears them in
-- the transaction that deletes the card, and nothing a player reads is built
-- from them.
--
-- A deck's hero page (holder_entity_id) is cleared, not cascaded, when the
-- page is purged: losing a hero page must not delete the table's deck. A copy
-- held by a purged page goes with the page; a later copies change for its
-- card adds the missing copy back to the pile.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS decks (
    id               CHAR(36)     NOT NULL,
    campaign_id      CHAR(36)     NOT NULL,
    name             VARCHAR(120) NOT NULL,
    kind             VARCHAR(8)   NOT NULL DEFAULT 'event',
    holder_entity_id CHAR(36)     NULL,
    next_card_id     CHAR(36)     NULL,
    last_card_id     CHAR(36)     NULL,
    settings         TEXT         NOT NULL,
    look             TEXT         NOT NULL,
    version          INT          NOT NULL DEFAULT 1,
    sort_order       INT          NOT NULL DEFAULT 0,
    created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_decks_name (campaign_id, name),
    KEY idx_decks_holder (holder_entity_id),
    CONSTRAINT fk_decks_campaign FOREIGN KEY (campaign_id)      REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_decks_holder   FOREIGN KEY (holder_entity_id) REFERENCES entities(id)  ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- The players a Director chose for a deck. draws_left NULL means no limit.
-- created_at is when the Director chose the player; a grant counts only while
-- it is not before the member's joined_at, so a player removed and invited
-- back is not silently restored. The service enforces it on read.
CREATE TABLE IF NOT EXISTS deck_players (
    deck_id    CHAR(36)   NOT NULL,
    user_id    CHAR(36)   NOT NULL,
    draws_left INT        NULL,
    created_at DATETIME   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (deck_id, user_id),
    KEY idx_deck_players_user (user_id),
    CONSTRAINT fk_deck_players_deck FOREIGN KEY (deck_id) REFERENCES decks(id) ON DELETE CASCADE,
    CONSTRAINT fk_deck_players_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- A card definition. words is the face a player reads; director_note and
-- when_json are never sent to players. fate '' means the deck's default.
CREATE TABLE IF NOT EXISTS deck_cards (
    id            CHAR(36)     NOT NULL,
    deck_id       CHAR(36)     NOT NULL,
    campaign_id   CHAR(36)     NOT NULL,
    name          VARCHAR(120) NOT NULL,
    kind_key      VARCHAR(32)  NOT NULL DEFAULT '',
    words         TEXT         NOT NULL,
    director_note TEXT         NOT NULL,
    picture       VARCHAR(255) NOT NULL DEFAULT '',
    corner        VARCHAR(12)  NOT NULL DEFAULT '',
    rare          TINYINT(1)   NOT NULL DEFAULT 0,
    copies        INT          NOT NULL DEFAULT 1,
    fate          VARCHAR(8)   NOT NULL DEFAULT '',
    when_json     TEXT         NULL,
    when_open     TINYINT(1)   NOT NULL DEFAULT 0,
    ask_first     TINYINT(1)   NOT NULL DEFAULT 0,
    sort_order    INT          NOT NULL DEFAULT 0,
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_deck_cards_name (deck_id, name),
    KEY idx_deck_cards_campaign (campaign_id),
    KEY idx_deck_cards_picture (campaign_id, picture),
    CONSTRAINT fk_deck_cards_deck     FOREIGN KEY (deck_id)     REFERENCES decks(id)     ON DELETE CASCADE,
    CONSTRAINT fk_deck_cards_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One physical card. zone is pile, shown, hand, play, used or gone.
-- holder_entity_id is the hero page whose hand or play area holds the copy;
-- NULL in play means the whole table, and NULL in hand on a party deck is the
-- party's own hand. face_up is whether the holder sees the face; is_open is the
-- separate step of showing a face-up card in a hand to the table, so turning a
-- card over never tells anyone else anything. is_open implies face_up and is
-- cleared whenever a copy leaves a hand.
CREATE TABLE IF NOT EXISTS deck_copies (
    id               CHAR(36)    NOT NULL,
    deck_id          CHAR(36)    NOT NULL,
    campaign_id      CHAR(36)    NOT NULL,
    card_id          CHAR(36)    NOT NULL,
    zone             VARCHAR(8)  NOT NULL DEFAULT 'pile',
    holder_entity_id CHAR(36)    NULL,
    face_up          TINYINT(1)  NOT NULL DEFAULT 0,
    is_open          TINYINT(1)  NOT NULL DEFAULT 0,
    asked_at         DATETIME    NULL,
    dealt_by         CHAR(36)    NULL,
    dealt_at         DATETIME    NULL,
    turned_at        DATETIME    NULL,
    moved_at         DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_deck_copies_zone (deck_id, zone),
    KEY idx_deck_copies_holder (holder_entity_id),
    KEY idx_deck_copies_card (card_id),
    CONSTRAINT fk_deck_copies_deck     FOREIGN KEY (deck_id)          REFERENCES decks(id)      ON DELETE CASCADE,
    CONSTRAINT fk_deck_copies_campaign FOREIGN KEY (campaign_id)      REFERENCES campaigns(id)  ON DELETE CASCADE,
    CONSTRAINT fk_deck_copies_card     FOREIGN KEY (card_id)          REFERENCES deck_cards(id) ON DELETE CASCADE,
    CONSTRAINT fk_deck_copies_holder   FOREIGN KEY (holder_entity_id) REFERENCES entities(id)   ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One line per move, Directors only. The service keeps the newest 1000 lines
-- per deck. A copy gets a fresh id whenever it returns to the pile, so a line
-- is read by card_name (kept as written, so a renamed or deleted card still
-- reads), not by copy_id. card_id and copy_id carry no foreign key so a line
-- outlives what it names.
CREATE TABLE IF NOT EXISTS deck_log (
    id               BIGINT      NOT NULL AUTO_INCREMENT,
    deck_id          CHAR(36)    NOT NULL,
    campaign_id      CHAR(36)    NOT NULL,
    card_id          CHAR(36)    NULL,
    card_name        VARCHAR(120) NOT NULL DEFAULT '',
    copy_id          CHAR(36)    NULL,
    action           VARCHAR(16) NOT NULL,
    actor_user_id    CHAR(36)    NOT NULL,
    holder_entity_id CHAR(36)    NULL,
    seen             TINYINT(1)  NOT NULL DEFAULT 0,
    created_at       DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_deck_log_deck (deck_id, id),
    CONSTRAINT fk_deck_log_deck     FOREIGN KEY (deck_id)     REFERENCES decks(id)     ON DELETE CASCADE,
    CONSTRAINT fk_deck_log_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
