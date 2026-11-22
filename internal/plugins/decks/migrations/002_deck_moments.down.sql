DROP TABLE IF EXISTS deck_moments;
ALTER TABLE deck_cards DROP INDEX IF EXISTS idx_deck_cards_when;
ALTER TABLE deck_cards DROP COLUMN IF EXISTS fits_since;
ALTER TABLE deck_cards DROP COLUMN IF EXISTS said_at;
ALTER TABLE deck_cards DROP COLUMN IF EXISTS has_when;
