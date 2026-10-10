-- New keys store "chron_" plus 8 random hex chars (14 chars) as their lookup
-- prefix; the old VARCHAR(8) left 2 random chars, so key creation collided on
-- the unique index. Existing 8-char prefixes stay valid and keep working.
-- MODIFY to the same definition is a no-op when re-run.
ALTER TABLE api_keys MODIFY COLUMN key_prefix VARCHAR(32) NOT NULL;
