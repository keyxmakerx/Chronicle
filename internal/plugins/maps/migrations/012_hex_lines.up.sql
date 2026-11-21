-- Lines (road, river, trail) lie on top of a hex's land instead of replacing it.
-- Each column is a six-bit mask of the hex's sides the line leaves through, bit
-- i meaning neighbour i in the order hex.go's Neighbors() returns them. NULL is
-- "no such line on this hex"; 0 is "the line is here with no exits" (a dot).
-- The range 0..63 is enforced in Go, like terrain, so the column stays plain.
ALTER TABLE map_hex_cells
    ADD COLUMN IF NOT EXISTS road  TINYINT UNSIGNED NULL,
    ADD COLUMN IF NOT EXISTS river TINYINT UNSIGNED NULL,
    ADD COLUMN IF NOT EXISTS trail TINYINT UNSIGNED NULL;
