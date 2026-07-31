-- Reverse C-CALV4-BENCH-R2 [BR2-5]: drop the per-viewer Bench disclosure state
-- from calendar_active. Dropping it returns every viewer to the ruled default —
-- four closed sections, each stating one true line — which is the same state a
-- viewer with no row has. The preference is display-only, so nothing authored
-- is lost.

ALTER TABLE calendar_active
  DROP COLUMN IF EXISTS bench_sections;
