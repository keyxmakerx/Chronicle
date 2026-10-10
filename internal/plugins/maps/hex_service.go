package maps

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
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
// stroke sends terrain and piece only, so it cannot touch a hex's name or
// notes. Piece picks one of the terrain's looks (0..PieceCount-1); null is
// "Mix", where the viewer picks a stable look from the hex's position.
type UpdateHexCellInput struct {
	Col     int
	Row     int
	Terrain patch.Field[string]
	Piece   patch.Field[int]
	Name    patch.Field[string]
	Notes   patch.Field[string]
}

// UpdateHexLayerInput is a partial change to the layer row: an absent field
// keeps, an explicit null clears, a present value replaces. Null on the anchor
// means "the whole map"; null on fog_enabled means off; null on a travel figure
// restores its default. A value outside MinTravelMiles..MaxTravelMiles is
// refused. Art is the map's terrain style (display_settings hexes.art), here
// so a DM grant, who cannot save the map's settings, can change it from the
// Paint mode; null restores the default.
type UpdateHexLayerInput struct {
	AnchorDrawingID patch.Field[string]
	FogEnabled      patch.Field[bool]
	MilesPerHex     patch.Field[int]
	MilesPerDay     patch.Field[int]
	Art             patch.Field[string]
}

// HexLayerView is what a viewer receives: the layer and only the cells their
// role may see. Hidden is true when the layer's picture is hidden from this
// viewer; the layer then carries no anchor and no cells, and the client draws
// no hexes at all rather than falling back to the whole map.
//
// Art is the map's terrain style, sent here as well as with the page so a
// change reaches every open viewer with the next read. It is empty for a
// hidden layer.
type HexLayerView struct {
	Layer   HexLayer  `json:"layer"`
	Cells   []HexCell `json:"cells"`
	Version uint64    `json:"version"`
	Hidden  bool      `json:"hidden"`
	Art     string    `json:"art,omitempty"`
}

// HexPictures is the hex layer's view of the map's pictures. It is narrow on
// purpose: the layer may read one picture and straighten it, nothing else.
type HexPictures interface {
	// GetPicture returns a drawing by id, or nil when it does not exist.
	GetPicture(ctx context.Context, id string) (*Drawing, error)
	// IsShadowed reports whether a shadow wholly covers the picture for a
	// viewer of this role, using the same rule that withholds drawings.
	IsShadowed(ctx context.Context, d *Drawing, role int) (bool, error)
	// ClearRotation turns a picture upright. Hexes do not turn, so a picture
	// carrying them must not either. expected is the picture's UpdatedAt as the
	// caller read it, so a concurrent edit to the picture is a conflict.
	ClearRotation(ctx context.Context, mapID, id string, actor HexActor, expected time.Time) error
}

type drawingHexPictures struct{ svc DrawingService }

// NewHexPictures adapts the drawing service, so a rotation reset goes through
// the same write path (and the same live update to other viewers) as an edit
// from the picture bar.
func NewHexPictures(svc DrawingService) HexPictures { return &drawingHexPictures{svc: svc} }

func (p *drawingHexPictures) GetPicture(ctx context.Context, id string) (*Drawing, error) {
	d, err := p.svc.GetDrawing(ctx, id)
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
		return nil, nil
	}
	return d, err
}

func (p *drawingHexPictures) IsShadowed(ctx context.Context, d *Drawing, role int) (bool, error) {
	return p.svc.IsDrawingShadowed(ctx, d, role)
}

// ClearRotation runs the actor through the normal drawing write. Only an owner
// or DM grant reaches it, and a DM-granted player's member role is below the
// drawing gate's scribe floor, so a DM is lifted to owner for this one write:
// the hex layer already authorised the change, and refusing here would leave a
// rotated picture under the hexes. The optimistic-concurrency token still
// applies, so an edit that landed after the picture was read is a 409.
func (p *drawingHexPictures) ClearRotation(ctx context.Context, mapID, id string, actor HexActor, expected time.Time) error {
	role := actor.Role
	if actor.IsDM {
		role = permissions.RoleOwner
	}
	return p.svc.UpdateDrawing(ctx, id, mapID, actor.UserID, role, actor.IsDM,
		UpdateDrawingInput{Rotation: patch.Of(0.0), ExpectedUpdatedAt: &expected})
}

// HexLayerWriteResult is the outcome of a layer change.
type HexLayerWriteResult struct {
	Version         uint64  `json:"version"`
	AnchorDrawingID *string `json:"anchor_drawing_id"`
	FogEnabled      bool    `json:"fog_enabled"`
	MilesPerHex     int     `json:"miles_per_hex"`
	MilesPerDay     int     `json:"miles_per_day"`
	Art             string  `json:"art,omitempty"`
}

// HexFogCell names one hex in a reveal or hide request.
type HexFogCell struct {
	Col int
	Row int
}

// PartyMoveResult is the outcome of moving the party. Path runs from the old
// position to the new one inclusive (just the new one on first placement);
// every hex on it is explored by the time it is returned.
type PartyMoveResult struct {
	Version uint64   `json:"version"`
	Path    []HexKey `json:"path"`
}

// HexEventPublisher announces that a map's hexes changed. It carries a version
// and, when it is safe, the party's path, never any cell contents: clients
// refetch the filtered read, so the audience rules live in one place.
type HexEventPublisher interface {
	PublishHexChanged(campaignID, mapID string, version uint64, partyPath []HexKey)
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
	// UpdateLayer changes the layer row (today: which picture it covers) and
	// returns the new version. Owner or DM access only.
	UpdateLayer(ctx context.Context, campaignID, mapID string, actor HexActor, in UpdateHexLayerInput) (*HexLayerWriteResult, error)
	// RevealFog marks hexes explored or unexplored. Owner or DM access only.
	RevealFog(ctx context.Context, campaignID, mapID string, actor HexActor, cells []HexFogCell, explored bool) (*HexWriteResult, error)
	// ResetFog hides every hex again. Owner or DM access only.
	ResetFog(ctx context.Context, campaignID, mapID string, actor HexActor) (*HexWriteResult, error)
	// MoveParty moves the party, revealing radius 1 along the path the server
	// computes. Who may is the map's hexes.party_who setting.
	MoveParty(ctx context.Context, campaignID, mapID string, actor HexActor, to HexFogCell) (*PartyMoveResult, error)
	// FogMask makes the hex service a HexFogLookup for pins, drawings, events
	// and the map picture.
	FogMask(ctx context.Context, mapID string) (*FogMask, error)
	// SetPictures wires the lookup behind the anchor checks.
	SetPictures(p HexPictures)
	// SetEventPublisher wires the live-update publisher. Unwired, writes still
	// succeed and clients pick changes up on their next read.
	SetEventPublisher(pub HexEventPublisher)
	// SetMapLoader wires the lookup of a whole map, which the fog needs for the
	// picture's size, the grid size and the party rule. Unwired, the fog fails
	// closed and only owners and DMs may move the party.
	SetMapLoader(fn func(ctx context.Context, mapID string) (*Map, error))
	// SetPictureInvalidator wires the drop of cached map-picture answers, called
	// with the campaign id when fog changes who may fetch the original.
	SetPictureInvalidator(fn func(campaignID string))
	// SetMapLookup wires the map-to-campaign lookup behind the IDOR check.
	SetMapLookup(fn func(ctx context.Context, mapID string) (string, error))
	// SetDrawPolicyLookup wires the map's "who can draw" value.
	SetDrawPolicyLookup(fn func(ctx context.Context, mapID string) (string, error))
	// SetArtWriter wires the save of the map's terrain art into its display
	// settings. Unwired, a request to change the art fails.
	SetArtWriter(fn func(ctx context.Context, mapID, art string) error)
}

type hexService struct {
	repo       HexRepository
	pictures   HexPictures
	mapLookup  func(ctx context.Context, mapID string) (string, error)
	drawPolicy func(ctx context.Context, mapID string) (string, error)
	events     HexEventPublisher
	mapLoader  func(ctx context.Context, mapID string) (*Map, error)
	invalidate func(campaignID string)
	artWriter  func(ctx context.Context, mapID, art string) error
}

// NewHexService creates a new hex service.
func NewHexService(repo HexRepository) HexService {
	return &hexService{repo: repo}
}

func (s *hexService) SetPictures(p HexPictures) { s.pictures = p }

func (s *hexService) SetEventPublisher(pub HexEventPublisher) { s.events = pub }

func (s *hexService) SetMapLoader(fn func(ctx context.Context, mapID string) (*Map, error)) {
	s.mapLoader = fn
}

func (s *hexService) SetArtWriter(fn func(ctx context.Context, mapID, art string) error) {
	s.artWriter = fn
}

func (s *hexService) SetPictureInvalidator(fn func(campaignID string)) { s.invalidate = fn }

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
	hidden, stale, err := s.anchorState(ctx, mapID, layer, role)
	if err != nil {
		return nil, err
	}
	if hidden {
		// Only the version leaves: no anchor, fog, party or miles, so nothing
		// about the withheld picture or its layer can be read from the reply.
		l := DefaultHexLayer(mapID)
		l.Version = layer.Version
		return &HexLayerView{Layer: l, Cells: []HexCell{}, Version: layer.Version, Hidden: true}, nil
	}
	if stale {
		// An owner or DM keeps the whole-map fallback so they can see and fix
		// the layer; the stored anchor is left alone.
		layer.AnchorDrawingID = nil
	}
	cells, err := s.repo.ListCells(ctx, mapID)
	if err != nil {
		return nil, err
	}
	layer = VisiblePartyLayer(layer, cells, role)
	return &HexLayerView{Layer: layer, Cells: VisibleCells(layer, cells, role), Version: layer.Version, Art: s.artOf(ctx, mapID)}, nil
}

// artOf is the map's terrain art for the read. The art is not secret and the
// page already carries it, so a failed lookup sends nothing and the viewer
// keeps what it has rather than failing the whole read.
func (s *hexService) artOf(ctx context.Context, mapID string) string {
	if s.mapLoader == nil {
		return ""
	}
	m, err := s.mapLoader(ctx, mapID)
	if err != nil {
		return ""
	}
	return m.HexArt()
}

// anchorState says how a viewer must treat the layer's anchor. hidden: the
// layer is withheld entirely (the picture is dm_only or under a shadow for
// them, or the anchor is unusable and they cannot see dm_only). stale: the
// anchor is unusable but the viewer may see it, so they get the whole-map
// fallback. An unusable anchor reads as hidden to players because the picture
// may have been dm_only before it was deleted; only an explicit PUT with a null
// anchor shows the cells to them again.
func (s *hexService) anchorState(ctx context.Context, mapID string, layer HexLayer, role int) (hidden, stale bool, err error) {
	if layer.AnchorDrawingID == nil {
		return false, false, nil
	}
	anchor, err := s.anchorPicture(ctx, *layer.AnchorDrawingID)
	if err != nil {
		return false, false, err
	}
	privileged := permissions.CanSeeDmOnly(role)
	if !UsableAnchor(mapID, anchor) {
		return !privileged, privileged, nil
	}
	if AnchorHidesLayer(anchor, role) {
		return true, false, nil
	}
	if !privileged {
		shadowed, err := s.pictures.IsShadowed(ctx, anchor, role)
		if err != nil {
			// Fail closed: a picture that might be shadowed is not revealed.
			return false, false, err
		}
		if shadowed {
			return true, false, nil
		}
	}
	return false, false, nil
}

// anchorPicture loads the picture a layer is pinned to. Without a wired
// lookup it fails closed: guessing "not hidden" could show a hidden picture's
// hexes.
func (s *hexService) anchorPicture(ctx context.Context, id string) (*Drawing, error) {
	if s.pictures == nil {
		return nil, apperror.NewMissingContext()
	}
	return s.pictures.GetPicture(ctx, id)
}

func (s *hexService) UpdateLayer(ctx context.Context, campaignID, mapID string, actor HexActor, in UpdateHexLayerInput) (*HexLayerWriteResult, error) {
	if err := s.requireMapInCampaign(ctx, campaignID, mapID); err != nil {
		return nil, err
	}
	// Choosing what the hexes cover or whether land is fogged also decides who
	// can see them, so it is a DM decision even where scribes may paint.
	if !actor.IsDM {
		return nil, apperror.NewForbidden("only the owner or a DM can change the hex layer")
	}
	if !in.AnchorDrawingID.Present() && !in.FogEnabled.Present() && !in.MilesPerHex.Present() && !in.MilesPerDay.Present() && !in.Art.Present() {
		return nil, apperror.NewBadRequest("nothing to change")
	}
	art := ""
	if in.Art.Present() {
		art = HexArtDetailed
		if !in.Art.IsNull() {
			art, _ = in.Art.Get()
		}
		if !IsValidHexArt(art) {
			return nil, apperror.NewBadRequest("terrain art must be one of: real, detailed, simple")
		}
		if s.artWriter == nil {
			return nil, apperror.NewMissingContext()
		}
	}
	// Checked before anything is written, so a bad figure cannot leave a body
	// that also moved the anchor half applied.
	perHex, err := travelFigure(in.MilesPerHex, DefaultMilesPerHex, "miles per hex")
	if err != nil {
		return nil, err
	}
	perDay, err := travelFigure(in.MilesPerDay, DefaultMilesPerDay, "miles per day")
	if err != nil {
		return nil, err
	}
	res := &HexLayerWriteResult{}
	if in.AnchorDrawingID.Present() {
		anchor, rotated, expected, err := s.resolveAnchor(ctx, mapID, in.AnchorDrawingID)
		if err != nil {
			return nil, err
		}
		version, err := s.repo.SetAnchor(ctx, mapID, anchor)
		if err != nil {
			return nil, err
		}
		// Straightened only once the anchor is stored, so a failed pin never
		// leaves a picture turned for nothing.
		if rotated {
			if err := s.pictures.ClearRotation(ctx, mapID, *anchor, actor, expected); err != nil {
				return nil, err
			}
		}
		res.Version, res.AnchorDrawingID = version, anchor
	}
	if in.FogEnabled.Present() {
		on := false
		if !in.FogEnabled.IsNull() {
			on, _ = in.FogEnabled.Get()
		}
		version, err := s.repo.SetFog(ctx, mapID, on)
		if err != nil {
			return nil, err
		}
		res.Version = version
	}
	if perHex != nil || perDay != nil {
		version, err := s.repo.SetTravel(ctx, mapID, perHex, perDay)
		if err != nil {
			return nil, err
		}
		res.Version = version
	}
	if art != "" {
		if err := s.artWriter(ctx, mapID, art); err != nil {
			return nil, err
		}
		// The art lives in the map's settings; the version bump is what tells
		// open viewers to read it again.
		version, err := s.repo.BumpVersion(ctx, mapID)
		if err != nil {
			return nil, err
		}
		res.Version, res.Art = version, art
	}
	layer, err := s.layerOrDefault(ctx, mapID)
	if err != nil {
		return nil, err
	}
	res.AnchorDrawingID, res.FogEnabled = layer.AnchorDrawingID, layer.FogEnabled
	res.MilesPerHex, res.MilesPerDay = layer.MilesPerHex, layer.MilesPerDay
	s.afterWrite(campaignID, mapID, res.Version, nil, true)
	return res, nil
}

// travelFigure turns a travel field into the value to store: nil keeps the
// column, null restores the default, a number must be a sensible distance.
func travelFigure(f patch.Field[int], def int, what string) (*int, error) {
	if !f.Present() {
		return nil, nil
	}
	if f.IsNull() {
		return &def, nil
	}
	v, _ := f.Get()
	if v < MinTravelMiles || v > MaxTravelMiles {
		return nil, apperror.NewBadRequest(fmt.Sprintf("%s must be between %d and %d", what, MinTravelMiles, MaxTravelMiles))
	}
	return &v, nil
}

// resolveAnchor validates a requested anchor: null is the whole map, an id must
// name a picture on this map. rotated and expected carry what ClearRotation
// needs.
func (s *hexService) resolveAnchor(ctx context.Context, mapID string, f patch.Field[string]) (anchor *string, rotated bool, expected time.Time, err error) {
	if f.IsNull() {
		return nil, false, time.Time{}, nil
	}
	id, _ := f.Get()
	pic, err := s.anchorPicture(ctx, id)
	if err != nil {
		return nil, false, time.Time{}, err
	}
	// One message for a missing id, another map's picture and a drawing of
	// the wrong kind, so the answer cannot be used to probe other maps.
	if !UsableAnchor(mapID, pic) {
		return nil, false, time.Time{}, apperror.NewBadRequest("the hexes can only cover a picture on this map")
	}
	return &id, pic.Rotation != 0, pic.UpdatedAt, nil
}

// afterWrite runs once per successful write: it announces the change and, when
// the write changed who may fetch the original map picture, drops the cached
// answers so the next request re-decides. A request that changes many hexes
// publishes once.
func (s *hexService) afterWrite(campaignID, mapID string, version uint64, partyPath []HexKey, pictureAffected bool) {
	if pictureAffected && s.invalidate != nil {
		s.invalidate(campaignID)
	}
	if s.events != nil {
		s.events.PublishHexChanged(campaignID, mapID, version, partyPath)
	}
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
		allowed := multiline && (r == '\n' || r == '\t' || r == '\r')
		if unicode.IsControl(r) && !allowed {
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
	if !e.Terrain.Present() && !e.Piece.Present() && !e.Name.Present() && !e.Notes.Present() {
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
	if e.Piece.Present() {
		w.PieceSet = true
		if !e.Piece.IsNull() {
			p, _ := e.Piece.Get()
			// The upper bound for the hex's own terrain is checked once the
			// stored terrain is known (fitPieces); this bounds the column.
			if p < 0 || p >= HexPiecesPerTerrain {
				return HexCellWrite{}, apperror.NewBadRequest("piece is out of range")
			}
			w.Piece = &p
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

// fitPieces checks every piece against the terrain the hex will have after the
// write (the entry's own, else the stored one), so a piece never names a look
// its terrain does not offer. A terrain change that leaves the stored piece out
// of range (painting a road, clearing the hex) puts the hex back to Mix in the
// same write: the piece only ever described the old terrain.
func fitPieces(writes []HexCellWrite, stored map[HexKey]HexCell) error {
	for i := range writes {
		w := &writes[i]
		old, had := stored[HexKey{w.Col, w.Row}]
		var terrain *string
		if w.TerrainSet {
			terrain = w.Terrain
		} else if had {
			terrain = old.Terrain
		}
		count := 0
		if terrain != nil {
			count = PieceCount(*terrain)
		}
		if w.PieceSet {
			if w.Piece != nil && *w.Piece >= count {
				return apperror.NewBadRequest("that piece does not fit this hex's terrain")
			}
			continue
		}
		if w.TerrainSet && had && old.Piece != nil && *old.Piece >= count {
			w.PieceSet, w.Piece = true, nil
		}
	}
	return nil
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
	layer, err := s.layerOrDefault(ctx, mapID)
	if err != nil {
		return nil, err
	}
	// A scribe who cannot see the layer must not be able to write to it: the
	// write would confirm or alter hexes they are not meant to know about.
	if !actor.IsDM {
		hidden, _, err := s.anchorState(ctx, mapID, layer, actor.Role)
		if err != nil {
			return nil, err
		}
		if hidden {
			return nil, apperror.NewForbidden("you cannot paint hexes on this map")
		}
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
		if w.PieceSet {
			p.PieceSet, p.Piece = true, w.Piece
		}
		if w.NameSet {
			p.NameSet, p.Name = true, w.Name
		}
		if w.NotesSet {
			p.NotesSet, p.Notes = true, w.Notes
		}
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

	if err := fitPieces(writes, stored); err != nil {
		return nil, err
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
	s.afterWrite(campaignID, mapID, version, nil, false)
	return &HexWriteResult{Version: version, Updated: len(writes)}, nil
}
