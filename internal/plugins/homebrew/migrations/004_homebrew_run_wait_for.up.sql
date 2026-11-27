-- wait_for is the person a held run waits on when that is not a Director:
-- the player who makes a "The player rolls first" roll. NULL while the run
-- waits on a Director, or never waited. The index lets that player read the
-- rolls waiting on them alongside their own asks.
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE homebrew_runs
    ADD COLUMN IF NOT EXISTS wait_for CHAR(36) NULL;

CREATE INDEX IF NOT EXISTS idx_homebrew_runs_wait_for ON homebrew_runs (campaign_id, wait_for, created_at);
