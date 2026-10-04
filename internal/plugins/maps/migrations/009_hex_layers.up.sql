-- Hexes as a layer on any map. A map has at most one hex layer (turned on by
-- display_settings grid.type = hex); its cells are sparse, so an unpainted hex
-- costs no row. Both tables are created here so later slices (fog, party,
-- trips, pinning to a picture) add behaviour without a schema change.
--
-- anchor_drawing_id has no foreign key: the service checks it names an image
-- drawing on this map, and a picture deleted later must leave the layer in
-- place rather than cascade-delete the painted terrain.
-- updated_by has no foreign key for the same reason map_drawings.created_by has
-- none: a removed user must not take painted hexes with them.
CREATE TABLE IF NOT EXISTS map_hex_layers (
    map_id            VARCHAR(36)       NOT NULL PRIMARY KEY,
    anchor_drawing_id CHAR(36)          NULL,
    fog_enabled       TINYINT(1)        NOT NULL DEFAULT 0,
    party_col         INT               NULL,
    party_row         INT               NULL,
    miles_per_hex     SMALLINT UNSIGNED NOT NULL DEFAULT 6,
    miles_per_day     SMALLINT UNSIGNED NOT NULL DEFAULT 24,
    version           BIGINT UNSIGNED   NOT NULL DEFAULT 0,
    updated_at        DATETIME          NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    CONSTRAINT fk_map_hex_layers_map FOREIGN KEY (map_id) REFERENCES maps(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- terrain is a VARCHAR checked against an allowlist in Go, not an ENUM, so a new
-- terrain kind is a code change and never a migration. piece is the chosen look
-- within a terrain (NULL means Mix).
CREATE TABLE IF NOT EXISTS map_hex_cells (
    map_id     VARCHAR(36) NOT NULL,
    col        INT         NOT NULL,
    `row`      INT         NOT NULL,
    terrain    VARCHAR(16) NULL,
    piece      TINYINT     NULL,
    name       VARCHAR(120) NOT NULL DEFAULT '',
    notes      TEXT        NULL,
    explored   TINYINT(1)  NOT NULL DEFAULT 0,
    updated_by VARCHAR(36) NULL,
    updated_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (map_id, col, `row`),
    CONSTRAINT fk_map_hex_cells_map FOREIGN KEY (map_id) REFERENCES maps(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
