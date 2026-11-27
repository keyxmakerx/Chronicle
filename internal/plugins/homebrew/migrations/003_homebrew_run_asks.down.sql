-- Reverses 003_homebrew_run_asks. IF EXISTS so a second run is harmless.
DROP INDEX IF EXISTS idx_homebrew_runs_asker ON homebrew_runs;
ALTER TABLE homebrew_runs
    DROP COLUMN IF EXISTS answer,
    DROP COLUMN IF EXISTS ask_at;
