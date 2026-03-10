-- Add shared_with JSON column to notes for per-player sharing.
-- NULL = use is_shared flag (backward compatible).
-- Non-null = JSON array of user IDs this note is shared with.
-- When shared_with is set, is_shared is ignored for filtering.

ALTER TABLE notes ADD COLUMN shared_with JSON DEFAULT NULL
    COMMENT 'JSON array of user IDs this note is shared with (null = use is_shared flag)';
