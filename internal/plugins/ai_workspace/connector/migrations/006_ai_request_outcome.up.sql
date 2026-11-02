-- 006_ai_request_outcome: room for why an answered request's changes failed.
--
-- outcome held only counts ("2 applied, 1 failed"), so an AI could see that
-- something failed but not why. It now also names each failed change and
-- its reason, which needs more than 120 characters.
ALTER TABLE ai_connection_requests MODIFY COLUMN outcome VARCHAR(1000) NOT NULL DEFAULT '';
