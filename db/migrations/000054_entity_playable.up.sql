-- Marks a character as one a player can play, so the Characters page's
-- "Who plays whom" list can leave NPCs out. Claimed characters count via
-- owner_user_id, so no backfill is needed.
ALTER TABLE entities ADD COLUMN IF NOT EXISTS playable BOOLEAN NOT NULL DEFAULT FALSE;
