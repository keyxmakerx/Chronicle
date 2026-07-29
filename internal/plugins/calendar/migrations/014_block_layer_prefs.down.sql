-- Reverse C-CALV4-LAYERS-P9 [LYR-3]: drop the per-viewer Block layer set from
-- calendar_active. Dropping it returns every viewer to their host's seed, which
-- is the same state a viewer with no row has — the preference is display-only,
-- so nothing authored is lost.

ALTER TABLE calendar_active
  DROP COLUMN IF EXISTS block_layers;
