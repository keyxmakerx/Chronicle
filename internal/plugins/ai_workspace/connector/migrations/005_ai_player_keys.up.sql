-- 005_ai_player_keys: players may make their own AI key.
--
-- for_player marks a key a player made. It is fixed when the key is made,
-- so a key keeps reading as a player even if its maker later owns the
-- campaign: an owner's key is always made fresh by the owner. A player key
-- works only while the site admin and the campaign's owner both let players
-- use AI, and answers with exactly what that player can see.
ALTER TABLE ai_connections ADD COLUMN IF NOT EXISTS for_player BOOLEAN NOT NULL DEFAULT FALSE;
