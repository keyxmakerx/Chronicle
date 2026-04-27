-- Down migration is intentionally empty: there's no safe rollback for a
-- heal. We don't know which rows were 'planned' before — only that they
-- are 'active' now. Reverting blindly would re-break enabling.
SELECT 1;
