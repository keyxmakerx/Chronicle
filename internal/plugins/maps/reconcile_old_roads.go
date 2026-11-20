package maps

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// This file converts hexes painted with the old road terrain into road lines
// lying on land. A road used to be a terrain with no land under it, drawn by
// joining every neighbouring road hex. Lines are explicit per-side masks, so
// the conversion has to choose which of those joins become exits.

// minRoadCycle is the fewest hexes a kept loop may have. A loop of this size or
// more is a real ring road; anything smaller (a triangle where a road bends
// through three mutually adjacent hexes, a diamond, a five-ring) is an artefact
// of "join every neighbour" and is dropped.
const minRoadCycle = 6

// OldRoadConversion is the new state of one old road hex: its road mask and
// the land that replaces the road terrain (nil when no land can be inferred).
// The hex's piece is cleared, since the old road had none.
type OldRoadConversion struct {
	Col, Row int
	Road     int
	Terrain  *string
}

// OldRoadStore is the persistence surface ReconcileOldRoadHexes needs.
type OldRoadStore interface {
	// ListMapIDsWithOldRoads returns the maps that still hold a hex whose
	// terrain is the old road.
	ListMapIDsWithOldRoads(ctx context.Context) ([]string, error)
	// ConvertMap reads the map's cells, applies plan's result and bumps the
	// layer version, in one transaction, and returns how many hexes changed.
	ConvertMap(ctx context.Context, mapID string, plan func([]HexCell) []OldRoadConversion) (int, error)
}

// ReconcileOldRoadHexes turns every hex painted with the old road terrain into
// a land hex carrying a road line, one map per transaction, and returns how
// many maps and hexes it converted. It is idempotent because a converted map
// holds no road terrain any more, so a second run lists nothing and writes
// nothing. A failure on one map is logged and skipped so the rest still
// convert; the first such error is returned for the caller to log.
//
// Which joins become exits is fixed by one rule: the kept lines have no loop
// of fewer than six hexes. Edges are taken from the lowest combined degree up
// (a spanning forest first, then any edge closing a loop of six or more), so
// at a bend whose end hexes also touch, the shortcut between them is considered
// last and the middle hex stays on the road rather than becoming a stub.
func ReconcileOldRoadHexes(ctx context.Context, store OldRoadStore) (maps int, hexes int, err error) {
	ids, lerr := store.ListMapIDsWithOldRoads(ctx)
	if lerr != nil {
		return 0, 0, fmt.Errorf("maps.ReconcileOldRoadHexes: list maps: %w", lerr)
	}
	var firstErr error
	for _, id := range ids {
		n, cerr := store.ConvertMap(ctx, id, PlanOldRoadConversion)
		if cerr != nil {
			slog.Error("maps: old road conversion failed for one map; continuing",
				slog.String("map_id", id), slog.Any("error", cerr))
			if firstErr == nil {
				firstErr = fmt.Errorf("maps.ReconcileOldRoadHexes: map %s: %w", id, cerr)
			}
			continue
		}
		if n > 0 {
			maps++
			hexes += n
		}
	}
	return maps, hexes, firstErr
}

// hexLess orders hexes by row, then column, so every tie in the conversion
// breaks the same way on every run.
func hexLess(a, b HexKey) bool {
	if a.Row != b.Row {
		return a.Row < b.Row
	}
	return a.Col < b.Col
}

type roadEdge struct{ a, b HexKey } // a sorts before b

// PlanOldRoadConversion computes the conversion of every old road hex among
// cells, in row-then-column order. It is pure so the rule is testable without
// a database.
func PlanOldRoadConversion(cells []HexCell) []OldRoadConversion {
	road := map[HexKey]bool{}
	byKey := make(map[HexKey]HexCell, len(cells))
	for _, c := range cells {
		k := HexKey{c.Col, c.Row}
		byKey[k] = c
		if c.Terrain != nil && *c.Terrain == TerrainRoad {
			road[k] = true
		}
	}
	if len(road) == 0 {
		return nil
	}
	keys := make([]HexKey, 0, len(road))
	for k := range road {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return hexLess(keys[i], keys[j]) })

	exits := keptRoadExits(keys, road)
	land := inferRoadLand(keys, road, byKey)

	out := make([]OldRoadConversion, 0, len(keys))
	for _, k := range keys {
		mask := exits[k]
		if r := byKey[k].Road; r != nil {
			mask |= *r
		}
		out = append(out, OldRoadConversion{Col: k.Col, Row: k.Row, Road: mask, Terrain: land[k]})
	}
	return out
}

// keptRoadExits chooses the edges between neighbouring road hexes that become
// exits and returns each hex's mask of them.
func keptRoadExits(keys []HexKey, road map[HexKey]bool) map[HexKey]int {
	deg := make(map[HexKey]int, len(keys))
	var edges []roadEdge
	for _, k := range keys {
		for _, n := range Neighbors(k.Col, k.Row) {
			if !road[n] {
				continue
			}
			deg[k]++
			if hexLess(k, n) {
				edges = append(edges, roadEdge{k, n})
			}
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		wi, wj := deg[edges[i].a]+deg[edges[i].b], deg[edges[j].a]+deg[edges[j].b]
		if wi != wj {
			return wi < wj
		}
		if edges[i].a != edges[j].a {
			return hexLess(edges[i].a, edges[j].a)
		}
		return hexLess(edges[i].b, edges[j].b)
	})

	parent := make(map[HexKey]HexKey, len(keys))
	for _, k := range keys {
		parent[k] = k
	}
	var find func(HexKey) HexKey
	find = func(k HexKey) HexKey {
		for parent[k] != k {
			parent[k] = parent[parent[k]]
			k = parent[k]
		}
		return k
	}
	kept := map[HexKey][]HexKey{}
	keep := func(e roadEdge) {
		kept[e.a] = append(kept[e.a], e.b)
		kept[e.b] = append(kept[e.b], e.a)
	}
	var rejected []roadEdge
	for _, e := range edges {
		ra, rb := find(e.a), find(e.b)
		if ra == rb {
			rejected = append(rejected, e)
			continue
		}
		parent[ra] = rb
		keep(e)
	}
	// A rejected edge closes a loop of (shortest kept path + 1) hexes; it is
	// restored only when that loop is a real ring. Depth minRoadCycle-2 is
	// enough to tell: a path of fewer edges than that means a small loop.
	for _, e := range rejected {
		if !reachableWithin(kept, e.a, e.b, minRoadCycle-2) {
			keep(e)
		}
	}

	exits := make(map[HexKey]int, len(keys))
	for _, k := range keys {
		nb := Neighbors(k.Col, k.Row)
		for _, o := range kept[k] {
			for i, n := range nb {
				if n == o {
					exits[k] |= 1 << i
				}
			}
		}
	}
	return exits
}

// reachableWithin reports whether to is at most depth edges from from in g.
func reachableWithin(g map[HexKey][]HexKey, from, to HexKey, depth int) bool {
	seen := map[HexKey]bool{from: true}
	frontier := []HexKey{from}
	for d := 0; d < depth; d++ {
		var next []HexKey
		for _, k := range frontier {
			for _, n := range g[k] {
				if n == to {
					return true
				}
				if !seen[n] {
					seen[n] = true
					next = append(next, n)
				}
			}
		}
		frontier = next
	}
	return false
}

// inferRoadLand picks the land each old road hex gets: the most common terrain
// among its neighbours, ignoring other road hexes and towns (a town is a place,
// not ground to extend). Ties go to the earlier kind in hexTerrains. A hex with
// no usable neighbour takes land from already-decided road hexes beside it, one
// snapshot per pass so the result does not depend on visiting order, until a
// pass decides nothing; the rest get none.
func inferRoadLand(keys []HexKey, road map[HexKey]bool, byKey map[HexKey]HexCell) map[HexKey]*string {
	rank := make(map[string]int, len(hexTerrains))
	for i, t := range hexTerrains {
		rank[t] = i
	}
	decided := map[HexKey]*string{}
	for {
		snapshot := make(map[HexKey]*string, len(decided))
		for k, v := range decided {
			snapshot[k] = v
		}
		progress := false
		for _, k := range keys {
			if _, done := decided[k]; done {
				continue
			}
			counts := make([]int, len(hexTerrains))
			for _, n := range Neighbors(k.Col, k.Row) {
				var t string
				if road[n] {
					d := snapshot[n]
					if d == nil {
						continue
					}
					t = *d
				} else if c, ok := byKey[n]; ok && c.Terrain != nil {
					t = *c.Terrain
				} else {
					continue
				}
				if r, ok := rank[t]; ok && t != TerrainTown && t != TerrainRoad {
					counts[r]++
				}
			}
			best := -1
			for r, n := range counts {
				if n > 0 && (best < 0 || n > counts[best]) {
					best = r
				}
			}
			if best >= 0 {
				t := hexTerrains[best]
				decided[k] = &t
				progress = true
			}
		}
		if !progress {
			break
		}
	}
	return decided
}

// NewOldRoadStore returns the MariaDB-backed OldRoadStore.
func NewOldRoadStore(db *sql.DB) OldRoadStore {
	return &oldRoadStore{db: db}
}

type oldRoadStore struct {
	db *sql.DB
}

func (s *oldRoadStore) ListMapIDsWithOldRoads(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT DISTINCT map_id FROM map_hex_cells WHERE terrain = 'road' ORDER BY map_id")
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, apperror.NewInternal(err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

func (s *oldRoadStore) ConvertMap(ctx context.Context, mapID string, plan func([]HexCell) []OldRoadConversion) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, apperror.NewInternal(err)
	}
	defer func() { _ = tx.Rollback() }()

	// The layer row is locked first, like every other hex write, so a painter
	// cannot slip a change in between the read below and the writes.
	if err := bumpLayer(ctx, tx, mapID); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx,
		"SELECT "+hexCellColumns+" FROM map_hex_cells WHERE map_id = ? ORDER BY `row`, col", mapID)
	if err != nil {
		return 0, apperror.NewInternal(err)
	}
	cells, err := scanHexCells(rows)
	if err != nil {
		return 0, err
	}
	conv := plan(cells)
	if len(conv) == 0 {
		return 0, nil
	}
	// Only road, terrain and piece are set: explored, name, notes and
	// updated_by stay as they are. The terrain guard keeps a hex a concurrent
	// writer repainted from being overwritten.
	for _, c := range conv {
		if _, err := tx.ExecContext(ctx,
			"UPDATE map_hex_cells SET road = ?, terrain = ?, piece = NULL "+
				"WHERE map_id = ? AND col = ? AND `row` = ? AND terrain = 'road'",
			c.Road, c.Terrain, mapID, c.Col, c.Row); err != nil {
			return 0, apperror.NewInternal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, apperror.NewInternal(err)
	}
	return len(conv), nil
}
