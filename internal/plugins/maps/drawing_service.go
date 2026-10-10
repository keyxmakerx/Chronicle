package maps

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/concurrency"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// validDrawingTypes enumerates allowed drawing types.
var validDrawingTypes = map[string]bool{
	"freehand":  true,
	"rectangle": true,
	"ellipse":   true,
	"polygon":   true,
	"text":      true,
	"shadow":    true,
	"image":     true,
	// Annotations: see drawing_annotation.go.
	DrawingTypeArrow:     true,
	DrawingTypeHighlight: true,
	DrawingTypeStep:      true,
	DrawingTypeCallout:   true,
}

// validLayerTypes enumerates allowed layer types.
var validLayerTypes = map[string]bool{
	"background": true,
	"drawing":    true,
	"token":      true,
	"gm":         true,
	"fog":        true,
}

// imageURISchemePattern matches a leading URI scheme ("http:", "javascript:",
// "data:", ...). Every legitimate token image_path Chronicle itself ever
// writes is scheme-less: a bare filename ("wolf.png") or a root-relative
// path ("/media/<id>"), rendered by the map widget straight into an <img
// src>/Leaflet iconUrl. A scheme means the browser instead fetches (or, for
// javascript:/vbscript:, could try to run) whatever the token's placer typed.
var imageURISchemePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

// trimURLControlChars mirrors the WHATWG URL parser's own preprocessing: it
// strips leading/trailing C0 controls and space, then removes every ASCII
// tab, CR and LF wherever they appear. Browsers apply this before parsing a
// URL assigned to <img src>/Leaflet iconUrl, so a leading " http://…" or
// "\thttp://…" is fetched as plain http(s) even though it doesn't match the
// scheme checks byte-for-byte — the checks below must see what the browser
// will actually parse, not the raw stored bytes.
func trimURLControlChars(p string) string {
	p = strings.TrimFunc(p, func(r rune) bool { return r <= ' ' })
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, p)
}

// isExternalImagePath reports whether a token image_path would make the
// viewer's browser reach outside Chronicle: any URI scheme other than the
// self-contained "data:" form (this also catches "http://…"/"https://…",
// since their scheme prefix matches before the "//" is ever inspected), or a
// protocol-relative "//host/…" reference. A relative path never matches.
func isExternalImagePath(p string) bool {
	p = trimURLControlChars(p)
	if p == "" {
		return false
	}
	if strings.HasPrefix(p, "//") {
		return true
	}
	if scheme := imageURISchemePattern.FindString(p); scheme != "" {
		return !strings.EqualFold(strings.TrimSuffix(scheme, ":"), "data")
	}
	return false
}

// validateTokenImagePath rejects a token image_path that isn't a local
// reference, per isExternalImagePath. Shared by CreateToken and UpdateToken
// so both entry points enforce the same rule.
func validateTokenImagePath(path *string) error {
	if path == nil || !isExternalImagePath(*path) {
		return nil
	}
	return apperror.NewValidation("image_path must be a local reference, not an external URL")
}

// DrawingService defines business logic for drawings, tokens, layers, and fog.
//
// Mutation methods (Update*, Delete*) accept an optional optimistic-
// concurrency token. Layer and fog gain UpdatedAt on migration 006 so
// they participate in the same pattern as drawings/tokens. Fog mutations
// are short-lived and don't expose an Update path; only Delete carries
// the token in case a "delete this fog you saw at time T" call races a
// concurrent reset.
type DrawingService interface {
	// Drawing CRUD.
	//
	// CreateDrawing and UpdateDrawing enforce the map's "who can draw" setting
	// from the caller's campaign role (input.CallerRole / the role argument), so
	// the rule holds for the web UI, the sync API and imports alike. A role of 0
	// is refused: an unresolved caller never draws.
	CreateDrawing(ctx context.Context, input CreateDrawingInput) (*Drawing, error)
	GetDrawing(ctx context.Context, id string) (*Drawing, error)
	// mapID on the write methods is the authorization boundary from the URL
	// path: the object must belong to that map, mirroring the read-path guard
	// (audit-R2 Finding 2 — IDOR).
	//
	// isDM (owner or co-DM, CampaignContext.CanAuthorDmOnly) is separate from
	// role: only a DM may create, change or delete a shadow, while role keeps
	// driving the "who can draw" gate unchanged.
	//
	// UpdateDrawing follows DeleteDrawing's ownership rule: owners and DM
	// access change any drawing, a scribe only the ones actorID created.
	UpdateDrawing(ctx context.Context, id, mapID, actorID string, role int, isDM bool, input UpdateDrawingInput) error
	DeleteDrawing(ctx context.Context, id, mapID string, expectedUpdatedAt *time.Time, actorID string, role int, isDM bool) error
	// ListDrawings also withholds, from viewers subject to shadow hiding,
	// non-shadow drawings lying wholly under a shadow area.
	ListDrawings(ctx context.Context, mapID string, role int, userID string) ([]Drawing, error)
	// IsDrawingShadowed answers the same question for one drawing fetched by
	// id, so a by-id read cannot reveal what the list withholds.
	IsDrawingShadowed(ctx context.Context, d *Drawing, role int) (bool, error)
	// WithholdImages returns the drawings with the picture file removed, for a
	// viewer subject to fog, from the picture a fogged hex layer is pinned to
	// and from every picture reaching into unexplored hexes. Those files show
	// land under the fog, so they never reach them; the drawing itself stays so
	// the viewer can still place the hexes by its box.
	WithholdImages(ctx context.Context, mapID string, role int, ds []Drawing) ([]Drawing, error)
	// FogWithholdsMedia reports whether mediaID is the file of a picture on
	// mapID that WithholdImages withholds, so the media server can refuse the
	// file itself to the same viewers (MapService.IsShadowedMapImage).
	FogWithholdsMedia(ctx context.Context, mapID, mediaID string) (bool, error)

	// Token CRUD. isDM on the writes is CampaignContext.CanAuthorDmOnly: anyone
	// else is answered NotFound for a token they may not see (hidden, or under
	// unexplored hexes), the same as for a token that does not exist.
	CreateToken(ctx context.Context, input CreateTokenInput) (*Token, error)
	GetToken(ctx context.Context, id string) (*Token, error)
	UpdateToken(ctx context.Context, id, mapID string, isDM bool, input UpdateTokenInput) error
	UpdateTokenPosition(ctx context.Context, id, mapID string, isDM bool, input UpdateTokenPositionInput) error
	DeleteToken(ctx context.Context, id, mapID string, expectedUpdatedAt *time.Time) error
	// ListTokens also withholds, from viewers subject to fog, tokens standing
	// in unexplored hexes.
	ListTokens(ctx context.Context, mapID string, role int) ([]Token, error)
	// IsTokenHidden answers, for one token fetched by id, whether role may not
	// see it, so a by-id read cannot reveal what the list withholds.
	IsTokenHidden(ctx context.Context, t *Token, role int) (bool, error)

	// Layer CRUD.
	CreateLayer(ctx context.Context, input CreateLayerInput) (*Layer, error)
	GetLayer(ctx context.Context, id string) (*Layer, error)
	UpdateLayer(ctx context.Context, id, mapID string, input UpdateLayerInput) error
	DeleteLayer(ctx context.Context, id, mapID string, expectedUpdatedAt *time.Time) error
	ListLayers(ctx context.Context, mapID string) ([]Layer, error)

	// Fog CRUD.
	CreateFog(ctx context.Context, input CreateFogInput) (*FogRegion, error)
	// mapID is the authorization boundary from the URL path: the fog region
	// must belong to that map (SEC-IDOR-4), mirroring DeleteDrawing/Token/Layer.
	DeleteFog(ctx context.Context, id, mapID string) error
	ListFog(ctx context.Context, mapID string) ([]FogRegion, error)
	ResetFog(ctx context.Context, mapID string) error

	// ShadowAreas makes every DrawingService a ShadowLookup, checked at compile
	// time where it is wired.
	ShadowAreas(ctx context.Context, mapID string) ([]ShadowArea, error)

	// Wiring.
	SetEventPublisher(pub MapEventPublisher)
	SetMapLookup(fn func(ctx context.Context, mapID string) (string, error))
	// SetDrawPolicyLookup wires the per-map "who can draw" value (DrawWhoOwners
	// or DrawWhoScribes). Unwired, every map uses the default (scribes).
	SetDrawPolicyLookup(fn func(ctx context.Context, mapID string) (string, error))
	// SetMediaVerifier wires the check that a picture's media file belongs to
	// the map's campaign. Unwired, picture writes are refused.
	SetMediaVerifier(v MediaVerifier)
	// SetHexFogLookup wires the hex fog, so drawings under unexplored hexes are
	// withheld like those under a shadow. Unwired, no fog is known.
	SetHexFogLookup(l HexFogLookup)
}

// MapEventPublisher emits domain events when map resources change.
// Implemented by the WebSocket EventBus adapter in routes.go.
type MapEventPublisher interface {
	PublishDrawingEvent(eventType string, campaignID string, drawing *Drawing)
	PublishTokenEvent(eventType string, campaignID string, token *Token)
	// PublishTokenPositionEvent carries isHidden so the fast-drag path gates
	// hidden tokens the same way PublishTokenEvent does — a GM-only token's
	// live position must not reach non-GM clients while it's being dragged.
	// mapID lets the publisher apply the hex fog to the new position.
	PublishTokenPositionEvent(campaignID, mapID, tokenID string, x, y float64, isHidden bool)
	PublishLayerEvent(eventType string, campaignID string, layer *Layer)
	PublishFogEvent(eventType string, campaignID, mapID string, region *FogRegion)
	PublishMarkerEvent(eventType string, campaignID string, marker *Marker)
}

// NoopMapEventPublisher is a no-op implementation for tests.
type NoopMapEventPublisher struct{}

func (NoopMapEventPublisher) PublishDrawingEvent(string, string, *Drawing) {}
func (NoopMapEventPublisher) PublishTokenEvent(string, string, *Token)     {}
func (NoopMapEventPublisher) PublishTokenPositionEvent(string, string, string, float64, float64, bool) {
}
func (NoopMapEventPublisher) PublishLayerEvent(string, string, *Layer)           {}
func (NoopMapEventPublisher) PublishFogEvent(string, string, string, *FogRegion) {}
func (NoopMapEventPublisher) PublishMarkerEvent(string, string, *Marker)         {}

var _ ShadowLookup = DrawingService(nil)

// drawingService implements DrawingService.
type drawingService struct {
	repo      DrawingRepository
	events    MapEventPublisher
	mapLookup func(ctx context.Context, mapID string) (string, error) // returns campaignID
	// drawPolicy returns the map's "who can draw" value; nil means the default.
	drawPolicy func(ctx context.Context, mapID string) (string, error)
	// onShadowChange tells the map service a shadow was written, so cached
	// answers about shadowed map pictures are dropped.
	onShadowChange func(campaignID string)
	// media confirms a picture's file is an image of the map's campaign.
	media MediaVerifier
	// hexFog supplies the fog mask for drawings under unexplored hexes.
	hexFog HexFogLookup
}

func (s *drawingService) SetHexFogLookup(l HexFogLookup) { s.hexFog = l }

// errImageWiring is the cause logged when picture writes arrive before the
// media verifier is wired.
var errImageWiring = errors.New("maps: media verifier not configured")

// NewDrawingService creates a new drawing service.
func NewDrawingService(repo DrawingRepository) DrawingService {
	return &drawingService{repo: repo, events: NoopMapEventPublisher{}}
}

// SetEventPublisher sets the event publisher for real-time sync.
func (s *drawingService) SetEventPublisher(pub MapEventPublisher) {
	s.events = pub
}

// SetMapLookup sets the function used to resolve a map's campaign ID for events.
func (s *drawingService) SetMapLookup(fn func(ctx context.Context, mapID string) (string, error)) {
	s.mapLookup = fn
}

// SetDrawPolicyLookup sets the function used to read a map's draw gate.
func (s *drawingService) SetDrawPolicyLookup(fn func(ctx context.Context, mapID string) (string, error)) {
	s.drawPolicy = fn
}

// SetShadowChangeHook registers the callback run after every shadow write.
func (s *drawingService) SetShadowChangeHook(fn func(campaignID string)) {
	s.onShadowChange = fn
}

// shadowChanged runs the hook for a shadow or picture write: a picture can be
// the one a hex layer is pinned to, or move into the fog, and either changes
// which files the media server must refuse. An unresolved campaign is passed
// as "", which the receiver treats as "everything", so a failed lookup can
// never leave a stale "not shadowed" answer behind.
func (s *drawingService) shadowChanged(ctx context.Context, d *Drawing) {
	if s.onShadowChange == nil || d == nil || (d.DrawingType != DrawingTypeShadow && d.DrawingType != DrawingTypeImage) {
		return
	}
	s.onShadowChange(s.campaignForMap(ctx, d.MapID))
}

// SetMediaVerifier sets the media check used for picture drawings.
func (s *drawingService) SetMediaVerifier(v MediaVerifier) {
	s.media = v
}

// requireDrawAccess is the server-side half of the map's "who can draw"
// setting. Scribe is the floor either way (the routes already require it);
// "owners" raises it to Owner. It fails closed on a role of 0 and on a policy
// lookup error, so a broken lookup can never widen who may draw.
func (s *drawingService) requireDrawAccess(ctx context.Context, mapID string, role int) error {
	if role < permissions.RoleScribe {
		return apperror.NewForbidden("you cannot draw on this map")
	}
	who := DrawWhoScribes
	if s.drawPolicy != nil {
		w, err := s.drawPolicy(ctx, mapID)
		if err != nil {
			return err
		}
		who = w
	}
	if who == DrawWhoOwners && role < permissions.RoleOwner {
		return apperror.NewForbidden("only owners can draw on this map")
	}
	return nil
}

// campaignForMap resolves the campaign ID for event publishing.
func (s *drawingService) campaignForMap(ctx context.Context, mapID string) string {
	if s.mapLookup == nil {
		return ""
	}
	cid, _ := s.mapLookup(ctx, mapID)
	return cid
}

// --- Drawing ---

// CreateDrawing validates input and creates a new drawing.
func (s *drawingService) CreateDrawing(ctx context.Context, input CreateDrawingInput) (*Drawing, error) {
	if err := s.requireDrawAccess(ctx, input.MapID, input.CallerRole); err != nil {
		return nil, err
	}
	dt := strings.TrimSpace(input.DrawingType)
	if !validDrawingTypes[dt] {
		return nil, apperror.NewBadRequest("invalid drawing type: " + dt)
	}
	if dt == DrawingTypeShadow {
		if err := requireShadowAuthor(input.CallerIsDM); err != nil {
			return nil, err
		}
		input.FillAlpha = normalizeShadowAlpha(input.FillAlpha)
	}
	if len(input.Points) == 0 {
		return nil, apperror.NewBadRequest("points are required")
	}
	if err := checkPointsSize(input.Points); err != nil {
		return nil, err
	}
	if dt == DrawingTypeShadow {
		if err := validateShadowPoints(input.Points); err != nil {
			return nil, err
		}
	}
	input.Crop = dropNullJSON(input.Crop)
	if dt == DrawingTypeImage {
		if err := validateImageGeometry(input.Points, input.Rotation); err != nil {
			return nil, err
		}
		crop, err := validateCrop(input.Crop)
		if err != nil {
			return nil, err
		}
		input.Crop = crop
		imageID := ""
		if input.ImageID != nil {
			imageID = *input.ImageID
		}
		if err := s.verifyImageMedia(ctx, input.MapID, imageID); err != nil {
			return nil, err
		}
		input.FillAlpha = clampImageOpacity(input.FillAlpha)
	} else if err := rejectImageFields(input.ImageID, input.Crop); err != nil {
		return nil, err
	}

	vis := input.Visibility
	if vis == "" {
		vis = "everyone"
	}

	d := &Drawing{
		ID:          generateID(),
		MapID:       input.MapID,
		LayerID:     input.LayerID,
		DrawingType: dt,
		Points:      input.Points,
		StrokeColor: input.StrokeColor,
		StrokeWidth: input.StrokeWidth,
		FillColor:   input.FillColor,
		FillAlpha:   input.FillAlpha,
		TextContent: input.TextContent,
		FontSize:    input.FontSize,
		Rotation:    input.Rotation,
		Visibility:  vis,
		CreatedBy:   &input.CreatedBy,
		FoundryID:   input.FoundryID,
		ImageID:     input.ImageID,
		Crop:        input.Crop,
		SortOrder:   input.SortOrder,
	}

	if d.StrokeColor == "" {
		d.StrokeColor = "#000000"
	}
	if d.StrokeWidth <= 0 {
		d.StrokeWidth = 2.0
	}
	if input.Imported && d.DrawingType == "text" {
		// An export from before the text bound may hold any label; import keeps
		// it, cleaned, rather than failing the whole drawing.
		d.TextContent = cleanImportedLabel(d.TextContent)
	} else if err := validateDrawingContent(d, true); err != nil {
		return nil, err
	}

	if err := s.repo.CreateDrawing(ctx, d); err != nil {
		return nil, err
	}
	s.shadowChanged(ctx, d)
	s.events.PublishDrawingEvent("created", s.campaignForMap(ctx, d.MapID), d)
	return d, nil
}

// GetDrawing returns a drawing by ID.
func (s *drawingService) GetDrawing(ctx context.Context, id string) (*Drawing, error) {
	return s.repo.GetDrawing(ctx, id)
}

// UpdateDrawing validates input and updates a drawing.
func (s *drawingService) UpdateDrawing(ctx context.Context, id, mapID, actorID string, role int, isDM bool, input UpdateDrawingInput) error {
	if err := s.requireDrawAccess(ctx, mapID, role); err != nil {
		return err
	}
	d, err := s.repo.GetDrawing(ctx, id)
	if err != nil {
		return err
	}
	// IDOR guard (audit-R2 Finding 2): the object must belong to the map in the
	// URL path. NotFound (not Forbidden) so existence isn't leaked.
	if d.MapID != mapID {
		return apperror.NewNotFound("drawing not found")
	}
	if err := s.notFoundIfHiddenFrom(ctx, d, isDM); err != nil {
		return err
	}
	// Scribes may edit only their own drawings, matching delete.
	if !isDM && !canDeleteOwn(role, actorID, d.CreatedBy) {
		if d.Visibility == "dm_only" {
			return apperror.NewNotFound("drawing not found")
		}
		return apperror.NewForbidden("you can only change drawings you created")
	}
	if d.DrawingType == DrawingTypeShadow {
		if err := requireShadowAuthor(isDM); err != nil {
			return err
		}
	}

	if err := concurrency.Check(d.UpdatedAt, input.ExpectedUpdatedAt, "drawing"); err != nil {
		return err
	}

	// Load-merge-write: `d` is the row as stored, so every merge below
	// defaults to the stored value; only a key the caller actually sent
	// can change anything.
	d.Points = input.Points.Val(d.Points)
	d.StrokeColor = input.StrokeColor.Val(d.StrokeColor)
	d.StrokeWidth = input.StrokeWidth.Val(d.StrokeWidth)
	d.FillColor = input.FillColor.Ptr(d.FillColor)
	d.FillAlpha = input.FillAlpha.Val(d.FillAlpha)
	d.TextContent = input.TextContent.Ptr(d.TextContent)
	d.FontSize = input.FontSize.Ptr(d.FontSize)
	d.Rotation = input.Rotation.Val(d.Rotation)
	d.Visibility = input.Visibility.Val(d.Visibility)
	d.SortOrder = input.SortOrder.Val(d.SortOrder)

	if d.DrawingType == DrawingTypeImage {
		if err := s.mergeImageFields(ctx, d, input); err != nil {
			return err
		}
	} else if err := rejectImageFields(input.ImageID.Ptr(nil), input.Crop.Val(nil)); err != nil {
		return err
	}

	// A shadow keeps its shape and strength rules on every write, so an edit
	// cannot leave a box players cannot interpret or hide a pin unpredictably.
	if d.DrawingType == DrawingTypeShadow {
		if err := validateShadowPoints(d.Points); err != nil {
			return err
		}
		d.FillAlpha = normalizeShadowAlpha(d.FillAlpha)
	}
	if err := checkPointsSize(d.Points); err != nil {
		return err
	}
	if err := validateDrawingContent(d, input.TextContent.Present()); err != nil {
		return err
	}

	if err := s.repo.UpdateDrawing(ctx, d); err != nil {
		return err
	}
	s.shadowChanged(ctx, d)
	s.events.PublishDrawingEvent("updated", s.campaignForMap(ctx, d.MapID), d)
	return nil
}

// maxPointsBytes bounds the stored points JSON of any drawing, on create and
// on the merged row of every update.
const maxPointsBytes = 10000

func checkPointsSize(points json.RawMessage) error {
	if len(points) > maxPointsBytes {
		return apperror.NewBadRequest("too many points (maximum 10,000)")
	}
	return nil
}

// validateDrawingContent applies the annotation rules to an annotation, and
// the shared text bound to a label whose text is being written. A stored
// label longer than the bound keeps working until its text is edited.
func validateDrawingContent(d *Drawing, textWritten bool) error {
	if isAnnotationType(d.DrawingType) {
		return validateAnnotation(d)
	}
	if d.DrawingType == "text" && textWritten && d.TextContent != nil {
		t, err := validateDrawingText(d.TextContent)
		if err != nil {
			return err
		}
		d.TextContent = t
	}
	return nil
}

// mergeImageFields applies the picture-only fields of a partial update and
// re-checks every picture rule on the merged row, so an edit to one field
// cannot leave the whole picture invalid.
func (s *drawingService) mergeImageFields(ctx context.Context, d *Drawing, input UpdateDrawingInput) error {
	if input.ImageID.IsNull() {
		return apperror.NewBadRequest("a picture needs an image")
	}
	if id, ok := input.ImageID.Get(); ok {
		// Only a changed id needs the media lookup; re-sending the stored id
		// keeps working even if the file was since removed.
		if d.ImageID == nil || *d.ImageID != id {
			if err := s.verifyImageMedia(ctx, d.MapID, id); err != nil {
				return err
			}
		}
		d.ImageID = &id
	}
	d.Crop = input.Crop.Val(d.Crop)
	if input.Crop.IsNull() {
		d.Crop = nil
	}
	crop, err := validateCrop(d.Crop)
	if err != nil {
		return err
	}
	d.Crop = crop
	if err := validateImageGeometry(d.Points, d.Rotation); err != nil {
		return err
	}
	d.FillAlpha = clampImageOpacity(d.FillAlpha)
	return nil
}

// DeleteDrawing removes a drawing. Owners may delete any drawing; a lower
// role only one it created (nil creator is owner-only). A dm_only drawing the
// caller did not create answers NotFound, so a scribe cannot probe for hidden
// drawings. Shadows are DM-only on top of that.
func (s *drawingService) DeleteDrawing(ctx context.Context, id, mapID string, expectedUpdatedAt *time.Time, actorID string, role int, isDM bool) error {
	d, err := s.repo.GetDrawing(ctx, id)
	if err != nil {
		return err
	}
	if d.MapID != mapID { // IDOR guard (audit-R2 Finding 2)
		return apperror.NewNotFound("drawing not found")
	}
	if err := s.notFoundIfHiddenFrom(ctx, d, isDM); err != nil {
		return err
	}
	// A co-DM grant counts as DM here, whatever the member role.
	if !isDM && !canDeleteOwn(role, actorID, d.CreatedBy) {
		if d.Visibility == "dm_only" {
			return apperror.NewNotFound("drawing not found")
		}
		return apperror.NewForbidden("you can only delete drawings you created")
	}
	if d.DrawingType == DrawingTypeShadow {
		if err := requireShadowAuthor(isDM); err != nil {
			return err
		}
	}
	if err := concurrency.Check(d.UpdatedAt, expectedUpdatedAt, "drawing"); err != nil {
		return err
	}
	if err := s.repo.DeleteDrawing(ctx, id); err != nil {
		return err
	}
	s.shadowChanged(ctx, d)
	s.events.PublishDrawingEvent("deleted", s.campaignForMap(ctx, d.MapID), d)
	return nil
}

// ListDrawings returns all drawings for a map, filtered by role and user
// (S1 — matches ListMarkers).
func (s *drawingService) ListDrawings(ctx context.Context, mapID string, role int, userID string) ([]Drawing, error) {
	drawings, err := s.repo.ListDrawings(ctx, mapID, role, userID)
	if err != nil || !shadowHidingApplies(role) {
		return drawings, err
	}
	areas, err := s.ShadowAreas(ctx, mapID)
	if err != nil {
		// Fail closed: a drawing that might be under a shadow is not sent.
		return nil, err
	}
	drawings = filterDrawingsByShadow(areas, drawings)
	fog, err := fogFor(ctx, s.hexFog, mapID, role)
	if err != nil {
		// Fail closed, as for shadows.
		return nil, err
	}
	if fog == nil {
		return drawings, nil
	}
	kept := make([]Drawing, 0, len(drawings))
	for i := range drawings {
		if !fog.HidesDrawing(&drawings[i]) {
			kept = append(kept, drawings[i])
		}
	}
	return kept, nil
}

// WithholdImages implements DrawingService.
func (s *drawingService) WithholdImages(ctx context.Context, mapID string, role int, ds []Drawing) ([]Drawing, error) {
	fog, err := fogFor(ctx, s.hexFog, mapID, role)
	if err != nil {
		return nil, err
	}
	if fog == nil {
		return ds, nil
	}
	out := make([]Drawing, len(ds))
	copy(out, ds)
	for i := range out {
		if fog.WithholdsImageOf(&out[i]) {
			out[i].ImageID = nil
		}
	}
	return out, nil
}

// FogWithholdsMedia implements DrawingService. It reads every drawing of the
// map, as the owner would, because the question is about the file and not
// about one viewer's list. No fog lookup wired means no fog is known.
func (s *drawingService) FogWithholdsMedia(ctx context.Context, mapID, mediaID string) (bool, error) {
	if s.hexFog == nil || mediaID == "" {
		return false, nil
	}
	fog, err := s.hexFog.FogMask(ctx, mapID)
	if err != nil || fog == nil {
		return err != nil, err
	}
	drawings, err := s.repo.ListDrawings(ctx, mapID, permissions.RoleOwner, "")
	if err != nil {
		return true, err
	}
	for i := range drawings {
		d := &drawings[i]
		if d.ImageID != nil && *d.ImageID == mediaID && fog.WithholdsImageOf(d) {
			return true, nil
		}
	}
	return false, nil
}

// notFoundIfHiddenFrom answers NotFound, the same as a missing id, when a
// caller who is not a DM writes to a drawing they may not see (under a shadow
// or wholly in unexplored hexes). Otherwise the write's answer (success,
// Forbidden, Conflict) would confirm the drawing exists, and the write would
// change something they were never shown.
func (s *drawingService) notFoundIfHiddenFrom(ctx context.Context, d *Drawing, isDM bool) error {
	if isDM {
		return nil
	}
	hidden, err := s.IsDrawingShadowed(ctx, d, permissions.RolePlayer)
	if err != nil {
		return err
	}
	if hidden {
		return apperror.NewNotFound("drawing not found")
	}
	return nil
}

// ShadowAreas returns the boxes of every shadow on a map. It is on the
// concrete service (wired into the map service and the event publisher by
// type assertion) so the rule lives here once and the map service never needs
// the drawing repository.
func (s *drawingService) ShadowAreas(ctx context.Context, mapID string) ([]ShadowArea, error) {
	shadows, err := s.repo.ListShadows(ctx, mapID)
	if err != nil {
		return nil, err
	}
	areas := make([]ShadowArea, 0, len(shadows))
	for _, d := range shadows {
		if a, ok := shadowAreaFromDrawing(d); ok {
			areas = append(areas, a)
		}
	}
	return areas, nil
}

// DrawingVisibleTo reports whether a viewer may read d by the rule ListDrawings
// applies in SQL: DM-equivalents see everything; anyone else never a dm_only
// drawing, nor one whose visibility_rules leave them out. Shadows are a
// separate check (IsDrawingShadowed).
func DrawingVisibleTo(d *Drawing, role int, userID string) bool {
	if d == nil {
		return false
	}
	if permissions.CanSeeDmOnly(role) {
		return true
	}
	return d.Visibility != "dm_only" && visibilityRulesAdmit(d.VisibilityRules, userID)
}

// MarkerVisibleTo is DrawingVisibleTo for a marker: ListMarkers applies the
// same dm_only and visibility_rules predicate.
func MarkerVisibleTo(m *Marker, role int, userID string) bool {
	if m == nil {
		return false
	}
	if permissions.CanSeeDmOnly(role) {
		return true
	}
	return m.Visibility != "dm_only" && visibilityRulesAdmit(m.VisibilityRules, userID)
}

// TokenVisibleTo reports whether a viewer may read t by ListTokens' rule: a
// hidden token is for DM-equivalents only.
func TokenVisibleTo(t *Token, role int) bool {
	return t != nil && (!t.IsHidden || permissions.CanSeeDmOnly(role))
}

// visibilityRulesAdmit applies stored visibility_rules for a non-owner viewer.
// Rules that do not parse admit no one, so a damaged row hides rather than
// shows.
func visibilityRulesAdmit(raw *string, userID string) bool {
	if raw == nil {
		return true
	}
	var rules VisibilityRules
	if err := json.Unmarshal([]byte(*raw), &rules); err != nil {
		return false
	}
	return rules.Allows(userID)
}

// IsDrawingShadowed reports whether a viewer of this role must not receive d.
func (s *drawingService) IsDrawingShadowed(ctx context.Context, d *Drawing, role int) (bool, error) {
	if d == nil || !shadowHidingApplies(role) {
		return false, nil
	}
	areas, err := s.ShadowAreas(ctx, d.MapID)
	if err != nil {
		return true, err
	}
	if DrawingUnderShadow(areas, d) {
		return true, nil
	}
	fog, err := fogFor(ctx, s.hexFog, d.MapID, role)
	if err != nil {
		return true, err
	}
	return fog.HidesDrawing(d), nil
}

// validateShadowPoints requires exactly two finite corners; a shadow is a box,
// and anything else would be stored but could never be drawn or enforced.
func validateShadowPoints(raw json.RawMessage) error {
	pts, ok := parsePoints(raw)
	if !ok || len(pts) != 2 {
		return apperror.NewBadRequest("a shadow needs exactly two corner points")
	}
	return nil
}

// --- Token ---

// CreateToken validates input and creates a new token.
func (s *drawingService) CreateToken(ctx context.Context, input CreateTokenInput) (*Token, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("token name is required")
	}

	if input.X < 0 || input.X > 100 || input.Y < 0 || input.Y > 100 {
		return nil, apperror.NewBadRequest("token coordinates must be between 0 and 100")
	}
	if err := validateTokenImagePath(input.ImagePath); err != nil {
		return nil, err
	}

	t := &Token{
		ID:             generateID(),
		MapID:          input.MapID,
		LayerID:        input.LayerID,
		EntityID:       input.EntityID,
		Name:           name,
		ImagePath:      input.ImagePath,
		X:              input.X,
		Y:              input.Y,
		Width:          input.Width,
		Height:         input.Height,
		Rotation:       input.Rotation,
		Scale:          input.Scale,
		IsHidden:       input.IsHidden,
		IsLocked:       input.IsLocked,
		Bar1Value:      input.Bar1Value,
		Bar1Max:        input.Bar1Max,
		Bar2Value:      input.Bar2Value,
		Bar2Max:        input.Bar2Max,
		AuraRadius:     input.AuraRadius,
		AuraColor:      input.AuraColor,
		LightRadius:    input.LightRadius,
		LightDimRadius: input.LightDimRadius,
		LightColor:     input.LightColor,
		VisionEnabled:  input.VisionEnabled,
		VisionRange:    input.VisionRange,
		Elevation:      input.Elevation,
		StatusEffects:  input.StatusEffects,
		Flags:          input.Flags,
		CreatedBy:      &input.CreatedBy,
		FoundryID:      input.FoundryID,
	}

	if t.Width <= 0 {
		t.Width = 1.0
	}
	if t.Height <= 0 {
		t.Height = 1.0
	}
	if t.Scale <= 0 {
		t.Scale = 1.0
	}

	if err := s.repo.CreateToken(ctx, t); err != nil {
		return nil, err
	}
	s.events.PublishTokenEvent("created", s.campaignForMap(ctx, t.MapID), t)
	return t, nil
}

// GetToken returns a token by ID.
func (s *drawingService) GetToken(ctx context.Context, id string) (*Token, error) {
	return s.repo.GetToken(ctx, id)
}

// UpdateToken validates input and updates a token.
func (s *drawingService) UpdateToken(ctx context.Context, id, mapID string, isDM bool, input UpdateTokenInput) error {
	t, err := s.repo.GetToken(ctx, id)
	if err != nil {
		return err
	}
	if t.MapID != mapID { // IDOR guard (audit-R2 Finding 2)
		return apperror.NewNotFound("token not found")
	}
	if err := s.notFoundIfTokenHiddenFrom(ctx, t, isDM); err != nil {
		return err
	}

	if err := concurrency.Check(t.UpdatedAt, input.ExpectedUpdatedAt, "token"); err != nil {
		return err
	}

	// Load-merge-write: `t` is the row as stored, so every merge below
	// defaults to the stored value; only a key the caller actually sent
	// can change anything.
	if input.Name != "" {
		t.Name = input.Name
	}
	t.ImagePath = input.ImagePath.Ptr(t.ImagePath)
	// Validate only a newly-supplied image_path, never the merged/stored
	// value: per the partial-update contract, an update that omits
	// image_path must succeed even if a pre-existing stored value predates
	// this check (a legacy row, an older sync-API client) — the caller never
	// touched that field and shouldn't be blocked by it.
	if input.ImagePath.Present() {
		if err := validateTokenImagePath(t.ImagePath); err != nil {
			return err
		}
	}
	t.X = input.X.Val(t.X)
	t.Y = input.Y.Val(t.Y)
	t.Width = input.Width.Val(t.Width)
	t.Height = input.Height.Val(t.Height)
	t.Rotation = input.Rotation.Val(t.Rotation)
	t.Scale = input.Scale.Val(t.Scale)
	t.IsHidden = input.IsHidden.Val(t.IsHidden)
	t.IsLocked = input.IsLocked.Val(t.IsLocked)
	t.Bar1Value = input.Bar1Value.Ptr(t.Bar1Value)
	t.Bar1Max = input.Bar1Max.Ptr(t.Bar1Max)
	t.Bar2Value = input.Bar2Value.Ptr(t.Bar2Value)
	t.Bar2Max = input.Bar2Max.Ptr(t.Bar2Max)
	t.AuraRadius = input.AuraRadius.Ptr(t.AuraRadius)
	t.AuraColor = input.AuraColor.Ptr(t.AuraColor)
	t.LightRadius = input.LightRadius.Ptr(t.LightRadius)
	t.LightDimRadius = input.LightDimRadius.Ptr(t.LightDimRadius)
	t.LightColor = input.LightColor.Ptr(t.LightColor)
	t.VisionEnabled = input.VisionEnabled.Val(t.VisionEnabled)
	t.VisionRange = input.VisionRange.Ptr(t.VisionRange)
	t.Elevation = input.Elevation.Val(t.Elevation)
	t.StatusEffects = input.StatusEffects.Val(t.StatusEffects)
	t.Flags = input.Flags.Val(t.Flags)

	if err := s.repo.UpdateToken(ctx, t); err != nil {
		return err
	}
	s.events.PublishTokenEvent("updated", s.campaignForMap(ctx, t.MapID), t)
	return nil
}

// UpdateTokenPosition updates only the position (optimized for drag).
//
// When expectedUpdatedAt is provided, the call refetches the token first
// to enforce optimistic concurrency. Drag pipelines that fire many
// position updates per second can simply omit it and accept last-writer-
// wins; deliberate "drop here" actions can include it to detect
// cross-user collisions. The pre-fetch is skipped on the no-token path
// to keep the drag fast path on the same code shape as before.
func (s *drawingService) UpdateTokenPosition(ctx context.Context, id, mapID string, isDM bool, input UpdateTokenPositionInput) error {
	// Range-check only what the caller sent; the stored axis is already valid.
	for _, axis := range []patch.Field[float64]{input.X, input.Y} {
		if v, ok := axis.Get(); ok && (v < 0 || v > 100) {
			return apperror.NewBadRequest("token coordinates must be between 0 and 100")
		}
	}
	// Load once: needed for the IDOR guard (audit-R2 Finding 2) AND the optional
	// concurrency check (the drag path may omit ExpectedUpdatedAt, but the map
	// boundary must be enforced on every write).
	t, err := s.repo.GetToken(ctx, id)
	if err != nil {
		return err
	}
	if t.MapID != mapID {
		return apperror.NewNotFound("token not found")
	}
	if err := s.notFoundIfTokenHiddenFrom(ctx, t, isDM); err != nil {
		return err
	}
	if input.ExpectedUpdatedAt != nil {
		if err := concurrency.Check(t.UpdatedAt, input.ExpectedUpdatedAt, "token"); err != nil {
			return err
		}
	}
	x, y := input.X.Val(t.X), input.Y.Val(t.Y)
	if err := s.repo.UpdateTokenPosition(ctx, id, x, y); err != nil {
		return err
	}
	// Reuse the token loaded above (position update doesn't change its map) to
	// resolve the campaign for the event.
	s.events.PublishTokenPositionEvent(s.campaignForMap(ctx, t.MapID), t.MapID, id, x, y, t.IsHidden)
	return nil
}

// DeleteToken removes a token.
func (s *drawingService) DeleteToken(ctx context.Context, id, mapID string, expectedUpdatedAt *time.Time) error {
	t, err := s.repo.GetToken(ctx, id)
	if err != nil {
		return err
	}
	if t.MapID != mapID { // IDOR guard (audit-R2 Finding 2)
		return apperror.NewNotFound("token not found")
	}
	if err := concurrency.Check(t.UpdatedAt, expectedUpdatedAt, "token"); err != nil {
		return err
	}
	if err := s.repo.DeleteToken(ctx, id); err != nil {
		return err
	}
	s.events.PublishTokenEvent("deleted", s.campaignForMap(ctx, t.MapID), t)
	return nil
}

// ListTokens returns all tokens for a map, filtered by role and, for viewers
// subject to fog, by the hex fog. A failed fog lookup fails the list.
func (s *drawingService) ListTokens(ctx context.Context, mapID string, role int) ([]Token, error) {
	tokens, err := s.repo.ListTokens(ctx, mapID, role)
	if err != nil {
		return nil, err
	}
	fog, err := fogFor(ctx, s.hexFog, mapID, role)
	if err != nil {
		return nil, err
	}
	if fog == nil {
		return tokens, nil
	}
	kept := make([]Token, 0, len(tokens))
	for i := range tokens {
		if !fog.HidesToken(&tokens[i]) {
			kept = append(kept, tokens[i])
		}
	}
	return kept, nil
}

// IsTokenHidden implements DrawingService: a GM-only token, or one in
// unexplored hexes, is hidden from anyone below CanSeeDmOnly. A failed fog
// lookup answers hidden.
func (s *drawingService) IsTokenHidden(ctx context.Context, t *Token, role int) (bool, error) {
	if !TokenVisibleTo(t, role) {
		return true, nil
	}
	fog, err := fogFor(ctx, s.hexFog, t.MapID, role)
	if err != nil {
		return true, err
	}
	return fog.HidesToken(t), nil
}

// notFoundIfTokenHiddenFrom is notFoundIfHiddenFrom for tokens.
func (s *drawingService) notFoundIfTokenHiddenFrom(ctx context.Context, t *Token, isDM bool) error {
	if isDM {
		return nil
	}
	hidden, err := s.IsTokenHidden(ctx, t, permissions.RolePlayer)
	if err != nil {
		return err
	}
	if hidden {
		return apperror.NewNotFound("token not found")
	}
	return nil
}

// --- Layer ---

// CreateLayer validates input and creates a new layer.
func (s *drawingService) CreateLayer(ctx context.Context, input CreateLayerInput) (*Layer, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("layer name is required")
	}
	if !validLayerTypes[input.LayerType] {
		return nil, apperror.NewBadRequest("invalid layer type: " + input.LayerType)
	}

	l := &Layer{
		ID:        generateID(),
		MapID:     input.MapID,
		Name:      name,
		LayerType: input.LayerType,
		SortOrder: input.SortOrder,
		IsVisible: input.IsVisible,
		Opacity:   input.Opacity,
		IsLocked:  input.IsLocked,
	}
	if l.Opacity <= 0 {
		l.Opacity = 1.0
	}

	if err := s.repo.CreateLayer(ctx, l); err != nil {
		return nil, err
	}
	s.events.PublishLayerEvent("created", s.campaignForMap(ctx, l.MapID), l)
	return l, nil
}

// GetLayer returns a layer by ID.
func (s *drawingService) GetLayer(ctx context.Context, id string) (*Layer, error) {
	return s.repo.GetLayer(ctx, id)
}

// UpdateLayer validates input and updates a layer.
func (s *drawingService) UpdateLayer(ctx context.Context, id, mapID string, input UpdateLayerInput) error {
	l, err := s.repo.GetLayer(ctx, id)
	if err != nil {
		return err
	}
	if l.MapID != mapID { // IDOR guard (audit-R2 Finding 2)
		return apperror.NewNotFound("layer not found")
	}

	if err := concurrency.Check(l.UpdatedAt, input.ExpectedUpdatedAt, "layer"); err != nil {
		return err
	}

	// Load-merge-write: `l` is the row as stored, so every merge below
	// defaults to the stored value; only a key the caller actually sent
	// can change anything.
	if input.Name != "" {
		l.Name = input.Name
	}
	l.SortOrder = input.SortOrder.Val(l.SortOrder)
	l.IsVisible = input.IsVisible.Val(l.IsVisible)
	l.Opacity = input.Opacity.Val(l.Opacity)
	l.IsLocked = input.IsLocked.Val(l.IsLocked)

	if err := s.repo.UpdateLayer(ctx, l); err != nil {
		return err
	}
	s.events.PublishLayerEvent("updated", s.campaignForMap(ctx, l.MapID), l)
	return nil
}

// DeleteLayer removes a layer.
func (s *drawingService) DeleteLayer(ctx context.Context, id, mapID string, expectedUpdatedAt *time.Time) error {
	l, err := s.repo.GetLayer(ctx, id)
	if err != nil {
		return err
	}
	if l.MapID != mapID { // IDOR guard (audit-R2 Finding 2)
		return apperror.NewNotFound("layer not found")
	}
	if err := concurrency.Check(l.UpdatedAt, expectedUpdatedAt, "layer"); err != nil {
		return err
	}
	if err := s.repo.DeleteLayer(ctx, id); err != nil {
		return err
	}
	s.events.PublishLayerEvent("deleted", s.campaignForMap(ctx, l.MapID), l)
	return nil
}

// ListLayers returns all layers for a map.
func (s *drawingService) ListLayers(ctx context.Context, mapID string) ([]Layer, error) {
	return s.repo.ListLayers(ctx, mapID)
}

// --- Fog ---

// CreateFog validates input and creates a fog region.
func (s *drawingService) CreateFog(ctx context.Context, input CreateFogInput) (*FogRegion, error) {
	if len(input.Points) == 0 {
		return nil, apperror.NewBadRequest("fog points are required")
	}

	f := &FogRegion{
		ID:         generateID(),
		MapID:      input.MapID,
		Points:     input.Points,
		IsExplored: input.IsExplored,
	}

	if err := s.repo.CreateFog(ctx, f); err != nil {
		return nil, err
	}
	s.events.PublishFogEvent("created", s.campaignForMap(ctx, f.MapID), f.MapID, f)
	return f, nil
}

// DeleteFog removes a fog region. Looks the row up first so the delete
// event can carry mapID and so the handler still gets a 404 when the fog
// region doesn't exist (rather than a silent success).
func (s *drawingService) DeleteFog(ctx context.Context, id, mapID string) error {
	f, err := s.repo.GetFog(ctx, id)
	if err != nil {
		return err
	}
	// IDOR guard (SEC-IDOR-4): the fog region must belong to the map in the URL
	// path — DeleteDrawing/Token/Layer all enforce this and DeleteFog was the
	// lone omission. NotFound (not Forbidden) so existence isn't leaked.
	if f.MapID != mapID {
		return apperror.NewNotFound("fog region not found")
	}
	if err := s.repo.DeleteFog(ctx, id); err != nil {
		return err
	}
	s.events.PublishFogEvent("deleted", s.campaignForMap(ctx, f.MapID), f.MapID, f)
	return nil
}

// ListFog returns all fog regions for a map.
func (s *drawingService) ListFog(ctx context.Context, mapID string) ([]FogRegion, error) {
	return s.repo.ListFog(ctx, mapID)
}

// ResetFog removes all fog regions for a map.
func (s *drawingService) ResetFog(ctx context.Context, mapID string) error {
	if err := s.repo.ResetFog(ctx, mapID); err != nil {
		return err
	}
	s.events.PublishFogEvent("reset", s.campaignForMap(ctx, mapID), mapID, nil)
	return nil
}
