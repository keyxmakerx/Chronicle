-- 000034: Add rich text support to calendar events.
-- Renames `description` to store ProseMirror JSON and adds `description_html`
-- for pre-rendered HTML (same dual-storage pattern as entity entries and notes).
ALTER TABLE calendar_events
    ADD COLUMN description_html TEXT DEFAULT NULL AFTER description;
