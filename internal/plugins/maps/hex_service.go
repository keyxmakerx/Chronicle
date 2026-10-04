package maps

import (
	"context"
	"unicode"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// HexActor is who is writing, as the handler resolved it. IsDM is the
// owner-or-DM-grant capability (CanAuthorDmOnly); Role is the member's actual
// role, which is what the "who can draw" gate compares, so a DM grant does not
// have to be a scribe to paint.
type HexActor struct {
	UserID string
	Role   int
	IsDM   bool
}

// UpdateHexCellInput is one entry of a hex batch. Col and Row name the hex and
// are required; every other field is presence-aware: absent keeps the stored
// value, an explicit null clears it, a present value replaces it. A paint
// stroke sends only terrain, so it cannot touch a hex's name or notes.
type UpdateHexCellInput struct {
	Col     int
	Row     int
	Terrain patch.Field[string]
	Name    patch.Field[string]
	Notes   patch.Field[string]
}

// HexLayerView is what a viewer receives: the layer and only the cells their
// role may see.
type HexLayerView struct {
	Layer   HexLayer  `json:"layer"`
	Cells   []HexCell `json:"cells"`
	Version uint64    `json:"version"`
}

// HexWriteResult is the outcome of a batch.
type HexWriteResult struct {
	Version uint64 `json:"version"`
	Updated int    `json:"updated"`
}

// HexService is the hex layer's business logic.
type HexService interface {
	// GetLayer returns the layer with the cells the role may see. role is the
	// viewer's VisibilityRole.
	GetLayer(ctx context.Context, campaignID, mapID string, role int) (*HexLayerView, error)
	// PatchCells applies a batch of partial cell changes.
	PatchCells(ctx context.Context, campaignID, mapID string, actor HexActor, entries []UpdateHexCellInput) (*HexWriteResult, error)
	// SetMapLookup wires the map-to-campaign lookup behind the IDOR check.
	SetMapLookup(fn func(ctx context.Context, mapID string) (string, error))
	// SetDrawPolicyLookup wires the map's "who can draw" value.
	SetDrawPolicyLookup(fn func(ctx context.Context, mapID string) (string, error))
}

type hexService struct {
	repo       HexRepository
	mapLookup  func(ctx context.Context, mapID string) (string, error)
	drawPolicy func(ctx context.Context, mapID string) (string, error)
}

// NewHexService creates a new hex service.
func NewHexService(repo HexRepository) HexService {
	return &hexService{repo: repo}
}

func (s *hexService) SetMapLookup(fn func(ctx context.Context, mapID string) (string, error)) {
	s.mapLookup = fn
}

func (s *hexService) SetDrawPolicyLookup(fn func(ctx context.Context, mapID string) (string, error)) {
	s.drawPolicy = fn
}

// requireMapInCampaign is the IDOR guard: a map of another campaign answers
// NotFound, the same as a map that does not exist. Without a wired lookup it
// fails closed rather than trusting the path.
func (s *hexService) requireMapInCampaign(ctx context.Context, campaignID, mapID string) error {
	if s.mapLookup == nil {
		return apperror.NewMissingContext()
	}
	cid, err := s.mapLookup(ctx, mapID)
	if err != nil {
		return err
	}
	if cid != campaignID {
		return apperror.NewNotFound("map not found")
	}
	return nil
}

func (s *hexService) layerOrDefault(ctx context.Context, mapID string) (HexLayer, error) {
	l, err := s.repo.GetLayer(ctx, mapID)
	if err != nil {
		return HexLayer{}, err
	}
	if l == nil {
		return DefaultHexLayer(mapID), nil
	}
	return *l, nil
}

func (s *hexService) GetLayer(ctx context.Context, campaignID, mapID string, role int) (*HexLayerView, error) {
	if err := s.requireMapInCampaign(ctx, campaignID, mapID); err != nil {
		return nil, err
	}
	layer, err := s.layerOrDefault(ctx, mapID)
	if err != nil {
		return nil, err
	}
	cells, err := s.repo.ListCells(ctx, mapID)
	if err != nil {
		return nil, err
	}
	return &HexLayerView{Layer: layer, Cells: VisibleCells(layer, cells, role), Version: layer.Version}, nil
}

// requireWriter decides whether the actor may write hexes on this map at all.
// An owner or DM grant always may. A scribe may only when the map's draw policy
// lets scribes draw, since painting terrain is drawing on the map. Anyone else
// is refused. The policy lookup failing refuses too, so a broken lookup can
// never widen who may paint.
func (s *hexService) requireWriter(ctx context.Context, mapID string, a HexActor) error {
	if a.IsDM {
		return nil
	}
	if a.Role < permissions.RoleScribe {
		return apperror.NewForbidden("you cannot paint hexes on this map")
	}
	who := DrawWhoScribes
	if s.drawPolicy != nil {
		w, err := s.drawPolicy(ctx, mapID)
		if err != nil {
			return err
		}
		who = w
	}
	if who == DrawWhoOwners {
		return apperror.NewForbidden("only owners can paint hexes on this map")
	}
	return nil
}

// validHexText checks a name or note: valid UTF-8, within the rune cap, and
// free of control characters (notes may keep newlines and tabs). It is stored
// as plain text and escaped on output, so no markup is stripped here.
func validHexText(field, v string, maxRunes int, multiline bool) error {
	if !utf8.ValidString(v) {
		return apperror.NewBadRequest(field + " is not valid text")
	}
	if utf8.RuneCountInString(v) > maxRunes {
		return apperror.NewBadRequest(field + " is too long")
	}
	for _, r := range v {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\t' || r == '\r')) {
			return apperror.NewBadRequest(field + " has characters that cannot be used")
		}
	}
	return nil
}

// buildWrite validates one entry and turns it into a repository write.
func buildWrite(e UpdateHexCellInput) (HexCellWrite, error) {
	if e.Col < 0 || e.Row < 0 || e.Col > MaxHexCoord || e.Row > MaxHexCoord {
		return HexCellWrite{}, apperror.NewBadRequest("hex position is outside the map")
	}
	// An entry naming only a position changes nothing; refusing it keeps a
	// malformed client from burning a version bump (and a cap slot) on a no-op.
	if !e.Terrain.Present() && !e.Name.Present() && !e.Notes.Present() {
		return HexCellWrite{}, apperror.NewBadRequest("every hex needs a field to change")
	}
	w := HexCellWrite{Col: e.Col, Row: e.Row}
	if e.Terrain.Present() {
		w.TerrainSet = true
		if !e.Terrain.IsNull() {
			t, _ := e.Terrain.Get()
			if !IsValidTerrain(t) {
				return HexCellWrite{}, apperror.NewBadRequest("unknown terrain: " + t)
			}
			w.Terrain = &t
		}
	}
	if e.Name.Present() {
		w.NameSet = true
		if !e.Name.IsNull() {
			n, _ := e.Name.Get()
			if err := validHexText("name", n, MaxHexNameRunes, false); err != nil {
				return HexCellWrite{}, err
			}
			w.Name = n
		}
	}
	if e.Notes.Present() {
		w.NotesSet = true
		if !e.Notes.IsNull() {
			n, _ := e.Notes.Get()
			if err := validHexText("notes", n, MaxHexNotesRunes, true); err != nil {
				return HexCellWrite{}, err
			}
			w.Notes = &n
		}
	}
	return w, nil
}

func (s *hexService) PatchCells(ctx context.Context, campaignID, mapID string, actor HexActor, entries []UpdateHexCellInput) (*HexWriteResult, error) {
	if err := s.requireMapInCampaign(ctx, campaignID, mapID); err != nil {
		return nil, err
	}
	if err := s.requireWriter(ctx, mapID, actor); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, apperror.NewBadRequest("no hexes to change")
	}
	if len(entries) > MaxHexBatch {
		return nil, apperror.NewBadRequest("too many hexes in one request")
	}

	// Entries naming the same hex are folded into one write, later fields
	// winning, so a stroke that crosses a hex twice is applied once.
	writes := make([]HexCellWrite, 0, len(entries))
	index := make(map[HexKey]int, len(entries))
	for _, e := range entries {
		w, err := buildWrite(e)
		if err != nil {
			return nil, err
		}
		k := HexKey{w.Col, w.Row}
		i, seen := index[k]
		if !seen {
			index[k] = len(writes)
			writes = append(writes, w)
			continue
		}
		p := &writes[i]
		if w.TerrainSet {
			p.TerrainSet, p.Terrain = true, w.Terrain
		}
		if w.NameSet {
			p.NameSet, p.Name = true, w.Name
		}
		if w.NotesSet {
			p.NotesSet, p.Notes = true, w.Notes
		}
	}

	layer, err := s.layerOrDefault(ctx, mapID)
	if err != nil {
		return nil, err
	}

	keys := make([]HexKey, len(writes))
	for i, w := range writes {
		keys[i] = HexKey{w.Col, w.Row}
	}
	stored, err := s.repo.GetCells(ctx, mapID, keys)
	if err != nil {
		return nil, err
	}

	// With fog on, a scribe may only touch hexes the party has explored: a
	// write to an unexplored hex would let them confirm or alter a secret
	// they are not meant to see. Owners and DM grants are not limited.
	if layer.FogEnabled && !actor.IsDM {
		for _, k := range keys {
			if c, ok := stored[k]; !ok || !c.Explored {
				return nil, apperror.NewForbidden("you can only change hexes the party has explored")
			}
		}
	}

	// The size cap counts only hexes the map does not store yet, so editing or
	// clearing an existing hex always works, even at the cap; otherwise a full
	// map could never be trimmed back down.
	n, err := s.repo.CountCells(ctx, mapID)
	if err != nil {
		return nil, err
	}
	newKeys := 0
	for _, k := range keys {
		if _, ok := stored[k]; !ok {
			newKeys++
		}
	}
	if n+newKeys > MaxHexCellsPerMap {
		return nil, apperror.NewBadRequest("this map has too many painted hexes")
	}

	version, err := s.repo.ApplyCells(ctx, mapID, actor.UserID, writes)
	if err != nil {
		return nil, err
	}
	return &HexWriteResult{Version: version, Updated: len(writes)}, nil
}
