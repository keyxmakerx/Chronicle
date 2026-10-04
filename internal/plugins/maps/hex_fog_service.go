package maps

import (
	"context"
	"errors"
	"net/http"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// partyMoveAttempts bounds the retry when another mover wins the race for the
// party's position between reading it and applying the move.
const partyMoveAttempts = 3

func validHexCoord(col, row int) bool {
	return col >= 0 && row >= 0 && col <= MaxHexCoord && row <= MaxHexCoord
}

// requireFogWriter is the gate for reveal, hide and reset: fog decides what
// players may know, so it is an owner or DM act even where scribes may paint.
func requireFogWriter(a HexActor) error {
	if !a.IsDM {
		return apperror.NewForbidden("only the owner or a DM can reveal or hide hexes")
	}
	return nil
}

func (s *hexService) RevealFog(ctx context.Context, campaignID, mapID string, actor HexActor, cells []HexFogCell, explored bool) (*HexWriteResult, error) {
	if err := s.requireMapInCampaign(ctx, campaignID, mapID); err != nil {
		return nil, err
	}
	if err := requireFogWriter(actor); err != nil {
		return nil, err
	}
	if len(cells) == 0 {
		return nil, apperror.NewBadRequest("no hexes to change")
	}
	if len(cells) > MaxFogBatch {
		return nil, apperror.NewBadRequest("too many hexes in one request")
	}
	// Folded so a stroke that crosses a hex twice is one write.
	seen := make(map[HexKey]bool, len(cells))
	keys := make([]HexKey, 0, len(cells))
	for _, c := range cells {
		if !validHexCoord(c.Col, c.Row) {
			return nil, apperror.NewBadRequest("hex position is outside the map")
		}
		k := HexKey{Col: c.Col, Row: c.Row}
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	if explored {
		if err := s.checkCellCap(ctx, mapID, keys); err != nil {
			return nil, err
		}
	}
	version, err := s.repo.SetExplored(ctx, mapID, actor.UserID, keys, explored)
	if err != nil {
		return nil, err
	}
	s.afterWrite(campaignID, mapID, version, nil, false)
	return &HexWriteResult{Version: version, Updated: len(keys)}, nil
}

// checkCellCap refuses a write that would store more hexes than a map may hold.
// Only hexes the map does not store yet count, so a write that adds none always
// works, even at the cap.
func (s *hexService) checkCellCap(ctx context.Context, mapID string, keys []HexKey) error {
	stored, err := s.repo.GetCells(ctx, mapID, keys)
	if err != nil {
		return err
	}
	n, err := s.repo.CountCells(ctx, mapID)
	if err != nil {
		return err
	}
	fresh := 0
	for _, k := range keys {
		if _, ok := stored[k]; !ok {
			fresh++
		}
	}
	if n+fresh > MaxHexCellsPerMap {
		return apperror.NewBadRequest("this map has too many painted hexes")
	}
	return nil
}

func (s *hexService) ResetFog(ctx context.Context, campaignID, mapID string, actor HexActor) (*HexWriteResult, error) {
	if err := s.requireMapInCampaign(ctx, campaignID, mapID); err != nil {
		return nil, err
	}
	if err := requireFogWriter(actor); err != nil {
		return nil, err
	}
	version, err := s.repo.ResetExplored(ctx, mapID, actor.UserID)
	if err != nil {
		return nil, err
	}
	s.afterWrite(campaignID, mapID, version, nil, false)
	return &HexWriteResult{Version: version}, nil
}

// requirePartyMover decides who may move the party: an owner or DM grant
// always, a scribe only while the map's hexes.party_who lets scribes, and
// nobody else. An unwired or failing map lookup leaves only owners and DMs, so
// a broken lookup can never widen who moves the party.
func (s *hexService) requirePartyMover(ctx context.Context, mapID string, a HexActor) error {
	if a.IsDM {
		return nil
	}
	if a.Role < permissions.RoleScribe || s.mapLoader == nil {
		return apperror.NewForbidden("you cannot move the party on this map")
	}
	m, err := s.mapLoader(ctx, mapID)
	if err != nil {
		return err
	}
	if m.PartyWho() == PartyWhoOwners {
		return apperror.NewForbidden("only owners can move the party on this map")
	}
	return nil
}

// fieldBounds is how many columns and rows the field has, so a move cannot
// reveal or place the party outside it. Without a loaded map only the global
// coordinate limit applies.
func (s *hexService) fieldBounds(m *Map, layer HexLayer) (cols, rows int) {
	cols, rows = MaxHexCoord+1, MaxHexCoord+1
	geo, _, ok := s.fieldGeometry(m, layer, nil)
	if ok {
		cols, rows = geo.Cols, geo.Rows
	}
	return cols, rows
}

func (s *hexService) MoveParty(ctx context.Context, campaignID, mapID string, actor HexActor, to HexFogCell) (*PartyMoveResult, error) {
	if err := s.requireMapInCampaign(ctx, campaignID, mapID); err != nil {
		return nil, err
	}
	if err := s.requirePartyMover(ctx, mapID, actor); err != nil {
		return nil, err
	}
	if !validHexCoord(to.Col, to.Row) {
		return nil, apperror.NewBadRequest("hex position is outside the map")
	}
	target := HexKey{Col: to.Col, Row: to.Row}

	for attempt := 0; ; attempt++ {
		res, err := s.tryMoveParty(ctx, campaignID, mapID, actor, target)
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusConflict && attempt+1 < partyMoveAttempts {
			continue
		}
		return res, err
	}
}

func (s *hexService) tryMoveParty(ctx context.Context, campaignID, mapID string, actor HexActor, target HexKey) (*PartyMoveResult, error) {
	layer, err := s.layerOrDefault(ctx, mapID)
	if err != nil {
		return nil, err
	}
	// A scribe who cannot see the layer must not move something on it.
	if !actor.IsDM {
		hidden, _, err := s.anchorState(ctx, mapID, layer, actor.Role)
		if err != nil {
			return nil, err
		}
		if hidden {
			return nil, apperror.NewForbidden("you cannot move the party on this map")
		}
	}
	cols, rows := MaxHexCoord+1, MaxHexCoord+1
	if s.mapLoader != nil {
		m, err := s.mapLoader(ctx, mapID)
		if err != nil {
			return nil, err
		}
		cols, rows = s.fieldBounds(m, layer)
	}
	if target.Col >= cols || target.Row >= rows {
		return nil, apperror.NewBadRequest("hex position is outside the map")
	}

	var from *HexKey
	if layer.PartyCol != nil && layer.PartyRow != nil {
		from = &HexKey{Col: *layer.PartyCol, Row: *layer.PartyRow}
	}
	if from != nil && *from == target {
		return &PartyMoveResult{Version: layer.Version, Path: []HexKey{target}}, nil
	}

	// A scribe may follow ground the party has seen, not jump into the dark.
	if !actor.IsDM {
		near := append([]HexKey{target}, neighborKeys(target)...)
		stored, err := s.repo.GetCells(ctx, mapID, near)
		if err != nil {
			return nil, err
		}
		explored := func(k HexKey) bool { return stored[k].Explored }
		if !OnExploredFrontier(explored, target) {
			return nil, apperror.NewForbidden("you can only move the party onto explored land or next to it")
		}
	}

	path := []HexKey{target}
	if from != nil {
		path = HexLine(*from, target)
	}
	reveal := RevealZone(path, cols, rows)
	if err := s.checkCellCap(ctx, mapID, reveal); err != nil {
		return nil, err
	}
	version, err := s.repo.ApplyParty(ctx, mapID, actor.UserID, from, target, reveal)
	if err != nil {
		return nil, err
	}
	var safe []HexKey
	if s.partyPathSafe(ctx, mapID, layer) {
		safe = path
	}
	s.afterWrite(campaignID, mapID, version, safe, false)
	return &PartyMoveResult{Version: version, Path: path}, nil
}

func neighborKeys(k HexKey) []HexKey {
	n := Neighbors(k.Col, k.Row)
	return n[:]
}

// partyPathSafe decides whether the path may ride on the live event, which
// reaches every connected client. Every hex on it was just revealed, so fog is
// no obstacle; what remains is a layer pinned to a picture players cannot see,
// whose hex positions would otherwise give that picture away. Anything that
// cannot be confirmed leaves the path off: clients still get the new version
// and refetch.
func (s *hexService) partyPathSafe(ctx context.Context, mapID string, layer HexLayer) bool {
	if layer.AnchorDrawingID == nil {
		return true
	}
	hidden, _, err := s.anchorState(ctx, mapID, layer, permissions.RolePlayer)
	return err == nil && !hidden
}

// FogMask implements HexFogLookup.
func (s *hexService) FogMask(ctx context.Context, mapID string) (*FogMask, error) {
	layer, err := s.layerOrDefault(ctx, mapID)
	if err != nil {
		return nil, err
	}
	if !layer.FogEnabled {
		return nil, nil
	}
	// Fog is on and the geometry cannot be built: fail closed with an error
	// rather than answering "nothing is fogged".
	if s.mapLoader == nil {
		return nil, apperror.NewMissingContext()
	}
	m, err := s.mapLoader(ctx, mapID)
	if err != nil {
		return nil, err
	}
	d := ResolveDisplay(m, "")
	if d.GridType != GridHex || m.ImageWidth <= 0 || m.ImageHeight <= 0 {
		return nil, nil
	}
	var anchor *Drawing
	if layer.AnchorDrawingID != nil {
		anchor, err = s.anchorPicture(ctx, *layer.AnchorDrawingID)
		if err != nil {
			return nil, err
		}
	}
	geo, anchorID, ok := s.fieldGeometry(m, layer, anchor)
	if !ok {
		return nil, nil
	}
	keys, err := s.repo.ListExplored(ctx, mapID)
	if err != nil {
		return nil, err
	}
	explored := make(map[HexKey]bool, len(keys))
	for _, k := range keys {
		explored[k] = true
	}
	return &FogMask{
		Version: layer.Version, Geo: geo, MapW: float64(m.ImageWidth), MapH: float64(m.ImageHeight),
		Explored: explored, AnchorID: anchorID,
	}, nil
}

// fieldGeometry lays out the field the way the viewer does. A layer pinned to
// a usable picture is laid inside that picture's box; otherwise (including a
// stale anchor, which the viewer also reads as "the whole map") it covers the
// map. ok is false when the map has no picture or hexes are not its grid.
// The axes are clamped to the same 400 hexes the viewer allows.
func (s *hexService) fieldGeometry(m *Map, layer HexLayer, anchor *Drawing) (geo HexGeometry, anchorID string, ok bool) {
	if m == nil || m.ImageWidth <= 0 || m.ImageHeight <= 0 {
		return HexGeometry{}, "", false
	}
	d := ResolveDisplay(m, "")
	if d.GridType != GridHex {
		return HexGeometry{}, "", false
	}
	w, h := float64(m.ImageWidth), float64(m.ImageHeight)
	geo = NewHexGeometry(float64(d.GridSize), w, h)
	if anchor != nil && UsableAnchor(m.ID, anchor) {
		if pts, good := parsePoints(anchor.Points); good && len(pts) == 2 {
			bx, by := minFloat(pts[0].X, pts[1].X)/100*w, minFloat(pts[0].Y, pts[1].Y)/100*h
			bw, bh := absFloat(pts[1].X-pts[0].X)/100*w, absFloat(pts[1].Y-pts[0].Y)/100*h
			if g := NewAnchoredHexGeometry(float64(d.GridSize), bx, by, bw, bh); g.R > 0 {
				geo, anchorID = g, anchor.ID
			}
		}
	}
	geo.Cols = minInt(geo.Cols, MaxHexCoord+1)
	geo.Rows = minInt(geo.Rows, MaxHexCoord+1)
	return geo, anchorID, true
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
