package maps

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/concurrency"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// validDrawingTypes enumerates allowed drawing types.
var validDrawingTypes = map[string]bool{
	"freehand":  true,
	"rectangle": true,
	"ellipse":   true,
	"polygon":   true,
	"text":      true,
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
	UpdateDrawing(ctx context.Context, id, mapID string, role int, input UpdateDrawingInput) error
	DeleteDrawing(ctx context.Context, id, mapID string, expectedUpdatedAt *time.Time) error
	ListDrawings(ctx context.Context, mapID string, role int, userID string) ([]Drawing, error)

	// Token CRUD.
	CreateToken(ctx context.Context, input CreateTokenInput) (*Token, error)
	GetToken(ctx context.Context, id string) (*Token, error)
	UpdateToken(ctx context.Context, id, mapID string, input UpdateTokenInput) error
	UpdateTokenPosition(ctx context.Context, id, mapID string, input UpdateTokenPositionInput) error
	DeleteToken(ctx context.Context, id, mapID string, expectedUpdatedAt *time.Time) error
	ListTokens(ctx context.Context, mapID string, role int) ([]Token, error)

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

	// Wiring.
	SetEventPublisher(pub MapEventPublisher)
	SetMapLookup(fn func(ctx context.Context, mapID string) (string, error))
	// SetDrawPolicyLookup wires the per-map "who can draw" value (DrawWhoOwners
	// or DrawWhoScribes). Unwired, every map uses the default (scribes).
	SetDrawPolicyLookup(fn func(ctx context.Context, mapID string) (string, error))
}

// MapEventPublisher emits domain events when map resources change.
// Implemented by the WebSocket EventBus adapter in routes.go.
type MapEventPublisher interface {
	PublishDrawingEvent(eventType string, campaignID string, drawing *Drawing)
	PublishTokenEvent(eventType string, campaignID string, token *Token)
	// PublishTokenPositionEvent carries isHidden so the fast-drag path gates
	// hidden tokens the same way PublishTokenEvent does — a GM-only token's
	// live position must not reach non-GM clients while it's being dragged.
	PublishTokenPositionEvent(campaignID, tokenID string, x, y float64, isHidden bool)
	PublishLayerEvent(eventType string, campaignID string, layer *Layer)
	PublishFogEvent(eventType string, campaignID, mapID string, region *FogRegion)
	PublishMarkerEvent(eventType string, campaignID string, marker *Marker)
}

// NoopMapEventPublisher is a no-op implementation for tests.
type NoopMapEventPublisher struct{}

func (NoopMapEventPublisher) PublishDrawingEvent(string, string, *Drawing)                     {}
func (NoopMapEventPublisher) PublishTokenEvent(string, string, *Token)                         {}
func (NoopMapEventPublisher) PublishTokenPositionEvent(string, string, float64, float64, bool) {}
func (NoopMapEventPublisher) PublishLayerEvent(string, string, *Layer)                         {}
func (NoopMapEventPublisher) PublishFogEvent(string, string, string, *FogRegion)               {}
func (NoopMapEventPublisher) PublishMarkerEvent(string, string, *Marker)                       {}

// drawingService implements DrawingService.
type drawingService struct {
	repo      DrawingRepository
	events    MapEventPublisher
	mapLookup func(ctx context.Context, mapID string) (string, error) // returns campaignID
	// drawPolicy returns the map's "who can draw" value; nil means the default.
	drawPolicy func(ctx context.Context, mapID string) (string, error)
}

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
	if len(input.Points) == 0 {
		return nil, apperror.NewBadRequest("points are required")
	}
	if len(input.Points) > 10000 {
		return nil, apperror.NewBadRequest("too many points (maximum 10,000)")
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
	}

	if d.StrokeColor == "" {
		d.StrokeColor = "#000000"
	}
	if d.StrokeWidth <= 0 {
		d.StrokeWidth = 2.0
	}

	if err := s.repo.CreateDrawing(ctx, d); err != nil {
		return nil, err
	}
	s.events.PublishDrawingEvent("created", s.campaignForMap(ctx, d.MapID), d)
	return d, nil
}

// GetDrawing returns a drawing by ID.
func (s *drawingService) GetDrawing(ctx context.Context, id string) (*Drawing, error) {
	return s.repo.GetDrawing(ctx, id)
}

// UpdateDrawing validates input and updates a drawing.
func (s *drawingService) UpdateDrawing(ctx context.Context, id, mapID string, role int, input UpdateDrawingInput) error {
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

	if err := s.repo.UpdateDrawing(ctx, d); err != nil {
		return err
	}
	s.events.PublishDrawingEvent("updated", s.campaignForMap(ctx, d.MapID), d)
	return nil
}

// DeleteDrawing removes a drawing.
func (s *drawingService) DeleteDrawing(ctx context.Context, id, mapID string, expectedUpdatedAt *time.Time) error {
	d, err := s.repo.GetDrawing(ctx, id)
	if err != nil {
		return err
	}
	if d.MapID != mapID { // IDOR guard (audit-R2 Finding 2)
		return apperror.NewNotFound("drawing not found")
	}
	if err := concurrency.Check(d.UpdatedAt, expectedUpdatedAt, "drawing"); err != nil {
		return err
	}
	if err := s.repo.DeleteDrawing(ctx, id); err != nil {
		return err
	}
	s.events.PublishDrawingEvent("deleted", s.campaignForMap(ctx, d.MapID), d)
	return nil
}

// ListDrawings returns all drawings for a map, filtered by role and user
// (S1 — matches ListMarkers).
func (s *drawingService) ListDrawings(ctx context.Context, mapID string, role int, userID string) ([]Drawing, error) {
	return s.repo.ListDrawings(ctx, mapID, role, userID)
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
func (s *drawingService) UpdateToken(ctx context.Context, id, mapID string, input UpdateTokenInput) error {
	t, err := s.repo.GetToken(ctx, id)
	if err != nil {
		return err
	}
	if t.MapID != mapID { // IDOR guard (audit-R2 Finding 2)
		return apperror.NewNotFound("token not found")
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
func (s *drawingService) UpdateTokenPosition(ctx context.Context, id, mapID string, input UpdateTokenPositionInput) error {
	if input.X < 0 || input.X > 100 || input.Y < 0 || input.Y > 100 {
		return apperror.NewBadRequest("token coordinates must be between 0 and 100")
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
	if input.ExpectedUpdatedAt != nil {
		if err := concurrency.Check(t.UpdatedAt, input.ExpectedUpdatedAt, "token"); err != nil {
			return err
		}
	}
	if err := s.repo.UpdateTokenPosition(ctx, id, input.X, input.Y); err != nil {
		return err
	}
	// Reuse the token loaded above (position update doesn't change its map) to
	// resolve the campaign for the event.
	s.events.PublishTokenPositionEvent(s.campaignForMap(ctx, t.MapID), id, input.X, input.Y, t.IsHidden)
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

// ListTokens returns all tokens for a map, filtered by role.
func (s *drawingService) ListTokens(ctx context.Context, mapID string, role int) ([]Token, error) {
	return s.repo.ListTokens(ctx, mapID, role)
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
