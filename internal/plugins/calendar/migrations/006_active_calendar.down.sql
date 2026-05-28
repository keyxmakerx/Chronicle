-- Reverse C-CAL-V2-SHELL-FOUNDATION: drop the active-calendar pointer
-- table. On rollback, the V2 shell falls back to its
-- campaign-default behavior.

DROP TABLE IF EXISTS calendar_active;
