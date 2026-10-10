-- Narrowing would truncate (and collide) any long prefix minted since, which
-- would lock those keys out. Left as a no-op rollback; the wider column is
-- harmless to older code.
SELECT 1;
