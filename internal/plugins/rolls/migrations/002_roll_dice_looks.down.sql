ALTER TABLE campaign_roll_settings
    DROP COLUMN IF EXISTS players_pick,
    DROP COLUMN IF EXISTS dice_offered,
    DROP COLUMN IF EXISTS quality,
    DROP COLUMN IF EXISTS crit_fail,
    DROP COLUMN IF EXISTS crit_succ,
    DROP COLUMN IF EXISTS dice_colour,
    DROP COLUMN IF EXISTS dice_set;
