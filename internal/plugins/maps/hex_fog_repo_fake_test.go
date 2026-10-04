package maps

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// The fog and party half of fakeHexRepo. It mirrors the real repository's
// rules (version bump on every write, an emptied hex loses its row, a stale
// expected position is a Conflict) so service tests exercise the same contract.

func (r *fakeHexRepo) ensureLayer(mapID string) {
	if r.layer == nil {
		l := DefaultHexLayer(mapID)
		r.layer = &l
	}
}

func (r *fakeHexRepo) ListExplored(context.Context, string) ([]HexKey, error) {
	out := []HexKey{}
	for k, c := range r.cells {
		if c.Explored {
			out = append(out, k)
		}
	}
	return out, nil
}

func (r *fakeHexRepo) SetFog(_ context.Context, mapID string, enabled bool) (uint64, error) {
	r.ensureLayer(mapID)
	r.layer.FogEnabled = enabled
	r.layer.Version++
	return r.layer.Version, nil
}

func (r *fakeHexRepo) SetExplored(_ context.Context, mapID, _ string, keys []HexKey, explored bool) (uint64, error) {
	r.ensureLayer(mapID)
	r.layer.Version++
	r.exploredWrites = append(r.exploredWrites, keys)
	r.exploredValue = append(r.exploredValue, explored)
	for _, k := range keys {
		c := r.cells[k]
		c.Col, c.Row, c.Explored = k.Col, k.Row, explored
		if !explored && c.Terrain == nil && c.Name == "" && c.Notes == nil && c.Piece == nil {
			delete(r.cells, k)
			continue
		}
		r.cells[k] = c
	}
	return r.layer.Version, nil
}

func (r *fakeHexRepo) ResetExplored(_ context.Context, mapID, _ string) (uint64, error) {
	r.ensureLayer(mapID)
	r.layer.Version++
	r.resets++
	for k, c := range r.cells {
		c.Explored = false
		if c.Terrain == nil && c.Name == "" && c.Notes == nil && c.Piece == nil {
			delete(r.cells, k)
			continue
		}
		r.cells[k] = c
	}
	return r.layer.Version, nil
}

func (r *fakeHexRepo) ApplyParty(_ context.Context, mapID, _ string, from *HexKey, to HexKey, reveal []HexKey) (uint64, error) {
	r.ensureLayer(mapID)
	if r.conflicts > 0 {
		r.conflicts--
		return 0, apperror.NewConflict("the party was moved by someone else; try again")
	}
	r.layer.Version++
	r.partyMoves = append(r.partyMoves, partyMove{from: from, to: to, reveal: reveal})
	for _, k := range reveal {
		c := r.cells[k]
		c.Col, c.Row, c.Explored = k.Col, k.Row, true
		r.cells[k] = c
	}
	r.layer.PartyCol, r.layer.PartyRow = &to.Col, &to.Row
	return r.layer.Version, nil
}
