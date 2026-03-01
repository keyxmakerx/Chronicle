-- Sprint 6: Calendar modes + user timezone.
-- Adds mode column to calendars (reallife vs fantasy) and timezone to users.

-- Calendar mode: 'reallife' syncs to wall clock, 'fantasy' is fully custom.
ALTER TABLE calendars
  ADD COLUMN mode VARCHAR(20) NOT NULL DEFAULT 'fantasy' AFTER campaign_id;

-- User timezone for real-life calendar display and event creation.
ALTER TABLE users
  ADD COLUMN timezone VARCHAR(50) DEFAULT NULL;
