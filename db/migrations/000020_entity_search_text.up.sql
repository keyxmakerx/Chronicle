-- Add search_text column for expanded full-text search.
-- Combines stripped HTML entry content + custom field values into a single
-- searchable TEXT column with a FULLTEXT index. Widget-only game systems,
-- type labels, and custom fields are all searchable through this column.

ALTER TABLE entities
  ADD COLUMN search_text TEXT DEFAULT NULL AFTER entry_html;

-- Backfill: strip HTML tags from entry_html and JSON syntax from fields_data.
-- This is a best-effort backfill; Go code maintains search_text going forward.
UPDATE entities SET search_text = CONCAT(
  COALESCE(REGEXP_REPLACE(entry_html, '<[^>]*>', ' '), ''),
  ' ',
  COALESCE(REGEXP_REPLACE(CAST(fields_data AS CHAR), '["{}\\[\\]:,]', ' '), '')
);

ALTER TABLE entities
  ADD FULLTEXT INDEX ft_entities_search_text (search_text);
