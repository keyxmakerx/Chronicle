-- Reverting a data-fix is not safe — we can't know which rows were added
-- by the migration vs. by normal user action. This is a no-op rollback.
-- To undo, manually inspect campaign_addons and addons tables.
SELECT 1;
