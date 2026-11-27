-- Reverse 000069: both tables hold only pins this migration introduced, so
-- dropping them loses nothing else.
DROP TABLE IF EXISTS sidebar_pins;
DROP TABLE IF EXISTS campaign_choice_pins;
