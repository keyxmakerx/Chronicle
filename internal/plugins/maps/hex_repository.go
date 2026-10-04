package maps

import (
	"context"
	"database/sql"
	"errors"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// HexCellWrite is one cell change as the repository applies it. Each Set flag
// says whether the caller named that field: only named fields are written, so
// a write that mentions one field cannot disturb another, and the merge happens
// in a single statement instead of a read followed by a write that a second
// painter could slip between.
type HexCellWrite struct {
	Col, Row   int
	TerrainSet bool
	Terrain    *string // nil with TerrainSet clears
	NameSet    bool
	Name       string // "" with NameSet clears
	NotesSet   bool
	Notes      *string // nil with NotesSet clears
}

// HexRepository defines persistence for hex layers and their cells.
type HexRepository interface {
	// GetLayer returns the map's layer row, or nil with no error when nothing
	// has been written yet.
	GetLayer(ctx context.Context, mapID string) (*HexLayer, error)
	// ListCells returns every stored cell of a map, unfiltered. Role filtering
	// is the service's job (VisibleCells), never the query's.
	ListCells(ctx context.Context, mapID string) ([]HexCell, error)
	// GetCells returns the stored cells among keys, indexed by position.
	GetCells(ctx context.Context, mapID string, keys []HexKey) (map[HexKey]HexCell, error)
	// CountCells returns how many cells a map stores.
	CountCells(ctx context.Context, mapID string) (int, error)
	// SetAnchor points the layer at a picture (nil: the whole map), creating the
	// layer row on first use, bumps the version and returns it. Only the anchor
	// column is touched, so it cannot disturb fog, the party or the miles.
	SetAnchor(ctx context.Context, mapID string, anchor *string) (uint64, error)
	// ApplyCells writes the changes and bumps the layer's version in one
	// transaction, creating the layer row on first use, deleting any touched cell
	// that ends up empty, and returns the new
	// version. The layer row is touched first so concurrent writers to one map
	// queue behind it and every version is unique.
	ApplyCells(ctx context.Context, mapID, userID string, writes []HexCellWrite) (uint64, error)
}

// hexRepo implements HexRepository with MariaDB.
type hexRepo struct {
	db *sql.DB
}

// NewHexRepository creates a new hex repository.
func NewHexRepository(db *sql.DB) HexRepository {
	return &hexRepo{db: db}
}

func (r *hexRepo) GetLayer(ctx context.Context, mapID string) (*HexLayer, error) {
	var l HexLayer
	err := r.db.QueryRowContext(ctx, `
		SELECT map_id, anchor_drawing_id, fog_enabled, party_col, party_row,
			miles_per_hex, miles_per_day, version, updated_at
		FROM map_hex_layers WHERE map_id = ?`, mapID).Scan(
		&l.MapID, &l.AnchorDrawingID, &l.FogEnabled, &l.PartyCol, &l.PartyRow,
		&l.MilesPerHex, &l.MilesPerDay, &l.Version, &l.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return &l, nil
}

func scanHexCells(rows *sql.Rows) ([]HexCell, error) {
	defer rows.Close()
	out := []HexCell{}
	for rows.Next() {
		var c HexCell
		if err := rows.Scan(&c.Col, &c.Row, &c.Terrain, &c.Piece, &c.Name, &c.Notes,
			&c.Explored, &c.UpdatedBy, &c.UpdatedAt); err != nil {
			return nil, apperror.NewInternal(err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

// `row` is a reserved word in MariaDB, so every statement quotes it.
const hexCellColumns = "col, `row`, terrain, piece, name, notes, explored, updated_by, updated_at"

func (r *hexRepo) ListCells(ctx context.Context, mapID string) ([]HexCell, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+hexCellColumns+" FROM map_hex_cells WHERE map_id = ? ORDER BY `row`, col", mapID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return scanHexCells(rows)
}

func (r *hexRepo) GetCells(ctx context.Context, mapID string, keys []HexKey) (map[HexKey]HexCell, error) {
	out := make(map[HexKey]HexCell, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	// Row-value IN keeps this one indexed lookup however many keys there are
	// (a batch is capped at MaxHexBatch).
	q := "SELECT " + hexCellColumns + " FROM map_hex_cells WHERE map_id = ? AND (col, `row`) IN ("
	args := []any{mapID}
	for i, k := range keys {
		if i > 0 {
			q += ","
		}
		q += "(?,?)"
		args = append(args, k.Col, k.Row)
	}
	q += ")"
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	cells, err := scanHexCells(rows)
	if err != nil {
		return nil, err
	}
	for _, c := range cells {
		out[HexKey{c.Col, c.Row}] = c
	}
	return out, nil
}

func (r *hexRepo) CountCells(ctx context.Context, mapID string) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM map_hex_cells WHERE map_id = ?`, mapID).Scan(&n); err != nil {
		return 0, apperror.NewInternal(err)
	}
	return n, nil
}

func (r *hexRepo) ApplyCells(ctx context.Context, mapID, userID string, writes []HexCellWrite) (uint64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, apperror.NewInternal(err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO map_hex_layers (map_id, version) VALUES (?, 1)
		ON DUPLICATE KEY UPDATE version = version + 1`, mapID); err != nil {
		return 0, apperror.NewInternal(err)
	}

	var by *string
	if userID != "" {
		by = &userID
	}
	// Each flag pair makes the UPDATE branch keep the stored value unless the
	// caller named the field; the INSERT branch (a hex nobody has touched yet)
	// takes what was sent, with unnamed fields at their column defaults.
	const upsert = "INSERT INTO map_hex_cells (map_id, col, `row`, terrain, name, notes, updated_by) " +
		"VALUES (?, ?, ?, ?, ?, ?, ?) " +
		"ON DUPLICATE KEY UPDATE " +
		"terrain = IF(?, VALUES(terrain), terrain), " +
		"name = IF(?, VALUES(name), name), " +
		"notes = IF(?, VALUES(notes), notes), " +
		"updated_by = VALUES(updated_by)"
	for _, w := range writes {
		if _, err := tx.ExecContext(ctx, upsert,
			mapID, w.Col, w.Row, w.Terrain, w.Name, w.Notes, by,
			w.TerrainSet, w.NameSet, w.NotesSet); err != nil {
			return 0, apperror.NewInternal(err)
		}
	}

	// A hex whose every field has been cleared carries no information; keeping
	// the row would still count against the map's cell cap, so a cleared hex
	// could never free its slot. Only touched rows are checked, in this
	// transaction, so a concurrent writer's row is never swept by mistake.
	// explored and piece belong to later slices and keep a row alive.
	for _, w := range writes {
		if _, err := tx.ExecContext(ctx, "DELETE FROM map_hex_cells WHERE map_id = ? AND col = ? AND `row` = ? "+
			"AND terrain IS NULL AND name = '' AND notes IS NULL AND piece IS NULL AND explored = 0",
			mapID, w.Col, w.Row); err != nil {
			return 0, apperror.NewInternal(err)
		}
	}

	var version uint64
	if err := tx.QueryRowContext(ctx,
		`SELECT version FROM map_hex_layers WHERE map_id = ?`, mapID).Scan(&version); err != nil {
		return 0, apperror.NewInternal(err)
	}
	if err := tx.Commit(); err != nil {
		return 0, apperror.NewInternal(err)
	}
	return version, nil
}

func (r *hexRepo) SetAnchor(ctx context.Context, mapID string, anchor *string) (uint64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, apperror.NewInternal(err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO map_hex_layers (map_id, anchor_drawing_id, version) VALUES (?, ?, 1)
		ON DUPLICATE KEY UPDATE anchor_drawing_id = VALUES(anchor_drawing_id), version = version + 1`,
		mapID, anchor); err != nil {
		return 0, apperror.NewInternal(err)
	}
	var version uint64
	if err := tx.QueryRowContext(ctx,
		`SELECT version FROM map_hex_layers WHERE map_id = ?`, mapID).Scan(&version); err != nil {
		return 0, apperror.NewInternal(err)
	}
	if err := tx.Commit(); err != nil {
		return 0, apperror.NewInternal(err)
	}
	return version, nil
}
