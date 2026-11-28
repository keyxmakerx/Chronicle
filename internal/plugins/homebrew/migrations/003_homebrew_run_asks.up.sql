-- A run that reached an "Ask the Director" step waits there for an answer.
-- ask_at is the step it waits at (counted from 0), NULL for a run that never
-- asked; answer is the Director's answer as JSON (yes, no or a written
-- reply, and what the Director changed in the steps after the ask). The
-- asker is created_by, so the index lets a player read their own asks.
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE homebrew_runs
    ADD COLUMN IF NOT EXISTS ask_at SMALLINT NULL,
    ADD COLUMN IF NOT EXISTS answer TEXT NULL;

CREATE INDEX IF NOT EXISTS idx_homebrew_runs_asker ON homebrew_runs (campaign_id, created_by, created_at);
