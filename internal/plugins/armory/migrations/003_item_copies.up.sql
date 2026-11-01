-- 003_item_copies: copies of an item that remember something of their own.
--
-- The Armory counts an item per holder (a character's Has Item line, a
-- stash's line). A copy here is part of that count that remembers marks, a
-- quality, who made it and when, or when it goes off; the rest of the count
-- is plain. holder_kind is 'character' or 'stash' and holder_id the
-- character's entity id or the stash's id, which are not foreign keys
-- because they live in two tables; a holder that goes leaves rows nothing
-- reads, and the count they belong to caps what is shown. memory is JSON
-- the service cleans. armory_move_picks keeps which copy a waiting move
-- request asked for, so it is still the one that goes when approved.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS armory_item_copies (
    id             BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
    campaign_id    VARCHAR(36) NOT NULL,
    holder_kind    VARCHAR(12) NOT NULL,
    holder_id      VARCHAR(36) NOT NULL,
    item_entity_id VARCHAR(36) NOT NULL,
    quantity       INT         NOT NULL,
    memory         TEXT        NOT NULL,
    created_at     TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_armory_item_copies_holder (campaign_id, holder_kind, holder_id, item_entity_id),
    CONSTRAINT fk_armory_item_copies_item FOREIGN KEY (item_entity_id)
        REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_armory_item_copies_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS armory_move_picks (
    move_id BIGINT NOT NULL PRIMARY KEY,
    copy_id BIGINT NOT NULL,
    CONSTRAINT fk_armory_move_picks_move FOREIGN KEY (move_id)
        REFERENCES item_moves(id) ON DELETE CASCADE,
    CONSTRAINT fk_armory_move_picks_copy FOREIGN KEY (copy_id)
        REFERENCES armory_item_copies(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
