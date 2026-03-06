-- Add recap field to sessions for post-session summaries visible to all members.
ALTER TABLE sessions ADD COLUMN recap TEXT DEFAULT NULL AFTER notes_html;
ALTER TABLE sessions ADD COLUMN recap_html TEXT DEFAULT NULL AFTER recap;
