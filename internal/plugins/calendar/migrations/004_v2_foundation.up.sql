-- C-CAL-V2-SCHEMA-FOUNDATION (Wave 0 PR 2): event tier + multi-cal default uniqueness.
--
-- 1. event.tier column on calendar_events: slug reference to per-campaign
--    event_tier_definitions in campaigns.settings JSON. Nullable; service
--    layer reads default from campaign settings when NULL (no migration
--    backfill needed — matches the empty-means-default pattern used for
--    AccentColor / FontFamily / etc. in CampaignSettings).
--
-- 2. is_default partial unique constraint on calendars: V2 unlocks multi-
--    calendar (multiple rows per campaign_id), but exactly one row per
--    campaign may have is_default=1. MariaDB doesn't support PostgreSQL-
--    style partial indexes with WHERE clauses, so we use a virtual
--    generated column (`default_marker`) that's campaign_id when is_default=1
--    and NULL otherwise, then UNIQUE on that column — standard SQL treats
--    NULL as distinct, so any number of non-default rows coexist; only the
--    default row of each campaign collides on the UNIQUE constraint.
--
-- Per cordinator/decisions/2026-05-28-cal-timeline-v2-design.md §A2, §C1
-- + cordinator/dispatches/chronicle/C-CAL-V2-SCHEMA-FOUNDATION.md §1, §3.

ALTER TABLE calendar_events
  ADD COLUMN tier VARCHAR(64) DEFAULT NULL,
  ADD INDEX idx_calendar_events_tier (tier);

ALTER TABLE calendars
  ADD COLUMN default_marker VARCHAR(36) AS (IF(is_default = 1, campaign_id, NULL)) VIRTUAL,
  ADD UNIQUE INDEX idx_one_default_per_campaign (default_marker);
