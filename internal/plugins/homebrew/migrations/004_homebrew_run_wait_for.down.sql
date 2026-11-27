-- Reverses 004_homebrew_run_wait_for. IF EXISTS so a second run is harmless.
DROP INDEX IF EXISTS idx_homebrew_runs_wait_for ON homebrew_runs;
ALTER TABLE homebrew_runs
    DROP COLUMN IF EXISTS wait_for;
