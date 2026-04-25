-- Reverting a data-fix is not safe — we can't distinguish rows added by this
-- migration from rows added through normal user action (toggling sync-api on).
-- This is a no-op rollback. To undo, manually inspect campaign_addons.
SELECT 1;
