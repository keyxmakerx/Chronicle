-- 002_roll_dice_looks: the campaign's own 3D dice.
--
-- The Director picks the die (set and colour), the crit success and crit fail
-- animations and the detail (quality) once for the campaign. An empty string
-- means "the default", so a campaign that never opens the picker keeps rolling
-- the built-in look and a later change to the defaults reaches it. The names
-- are checked against the dice engine's lists by the service, not by the
-- schema, because that list grows without a migration.
--
-- dice_offered lists the sets a player may pick from, as comma-separated set
-- keys; empty means every set. The campaign's own set is always among them.
--
-- players_pick says whether each player may use their own dice for their own
-- rolls (kept in their browser); everyone else still sees the campaign's.
ALTER TABLE campaign_roll_settings
    ADD COLUMN IF NOT EXISTS dice_set     VARCHAR(24) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS dice_colour  VARCHAR(24) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS crit_succ    VARCHAR(24) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS crit_fail    VARCHAR(24) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS quality      VARCHAR(8)  NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS dice_offered VARCHAR(200) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS players_pick TINYINT(1)  NOT NULL DEFAULT 1;
