-- 003_tracker_players: the players a Director chose to see a tracker (and
-- optionally change it).
--
-- A tracker whose visibility is 'players' is seen by exactly the users listed
-- here, besides the Directors. can_change lets that player log changes; it
-- never lets them strike, rename, hide or remove. The rows are meaningful
-- only while the tracker's visibility is 'players': the service clears them
-- when a Director switches it back, and a read ignores them otherwise.
-- user_id cascades so a deleted account leaves no grant behind; a player who
-- merely leaves the campaign stops matching because campaign access fails
-- before any tracker is read.
--
-- created_at is when the Director granted the sight. A grant counts only while
-- created_at is not before the member's campaign_members.joined_at, so a player
-- who is removed and invited back does not silently regain sight or the right
-- to change; the Director chooses them again. The service enforces it on read.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS tracker_players (
    tracker_id  CHAR(36)   NOT NULL,
    campaign_id CHAR(36)   NOT NULL,
    user_id     CHAR(36)   NOT NULL,
    can_change  TINYINT(1) NOT NULL DEFAULT 0,
    created_at  DATETIME   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tracker_id, user_id),
    KEY idx_tracker_players_campaign (campaign_id),
    CONSTRAINT fk_tracker_players_tracker  FOREIGN KEY (tracker_id)  REFERENCES trackers(id)  ON DELETE CASCADE,
    CONSTRAINT fk_tracker_players_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_tracker_players_user     FOREIGN KEY (user_id)     REFERENCES users(id)     ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
