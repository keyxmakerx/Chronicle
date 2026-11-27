-- 003_roll_member_dice: each member's own dice, and the dice each roll was
-- thrown in.
--
-- campaign_roll_member_dice keeps one member's pick in one campaign with
-- their account, so it follows them to another device and every screen can
-- throw their rolls in it. An empty column follows the campaign's dice.
-- Whether a pick is honoured is decided each time from campaign_roll_settings
-- (players_pick and dice_offered), so narrowing the offer never needs this
-- table cleaned, and widening it again brings the pick back.
--
-- campaign_rolls.roller_dice is the dice a roll was thrown in, settled when it
-- was made (JSON: set, colour, critSucc, critFail, own). It is empty on rolls
-- made before rolls carried it, which screens throw in the campaign's dice.
CREATE TABLE IF NOT EXISTS campaign_roll_member_dice (
    campaign_id CHAR(36)    NOT NULL,
    user_id     CHAR(36)    NOT NULL,
    dice_set    VARCHAR(24) NOT NULL DEFAULT '',
    dice_colour VARCHAR(24) NOT NULL DEFAULT '',
    crit_succ   VARCHAR(24) NOT NULL DEFAULT '',
    crit_fail   VARCHAR(24) NOT NULL DEFAULT '',
    updated_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (campaign_id, user_id),
    KEY idx_campaign_roll_member_dice_user (user_id),
    CONSTRAINT fk_campaign_roll_member_dice_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_campaign_roll_member_dice_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE campaign_rolls
    ADD COLUMN IF NOT EXISTS roller_dice VARCHAR(160) NOT NULL DEFAULT '';
