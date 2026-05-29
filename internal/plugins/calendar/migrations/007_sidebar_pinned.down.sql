-- Reverse Wave 1.7A §G: drop sidebar pin column from calendar_active.

ALTER TABLE calendar_active
  DROP COLUMN sidebar_pinned;
