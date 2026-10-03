package maps

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/concurrency"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// colorPattern validates hex color values to prevent XSS injection via the
// color field which is rendered into CSS style attributes.
var colorPattern = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// pinCategories is the closed set of pin kinds. The map page's kind picker and
// the Foundry module's pin-type table both speak exactly these five, so a value
// outside the set could never be shown or round-tripped.
var pinCategories = map[string]struct{}{
	"location": {}, "danger": {}, "treasure": {}, "quest": {}, "note": {},
}

// validatePinCategory rejects a pin_category outside the allowed set. A nil
// pointer (no kind) is valid. It is applied only to a value the caller sent:
// a stored legacy value is left alone, so an unrelated edit of an old marker
// is never blocked by data written before the set was enforced.
func validatePinCategory(c *string) error {
	if c == nil {
		return nil
	}
	if _, ok := pinCategories[*c]; !ok {
		return apperror.NewValidation("pin_category must be one of: location, danger, treasure, quest, note")
	}
	return nil
}

// generateID creates a random UUID v4 string.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// MapService defines business logic for the maps plugin.
//
// Mutation methods that mutate row-level state (Update*, Delete*) accept
// an optional expectedUpdatedAt token for optimistic concurrency. Pass
// nil for last-writer-wins; pass the caller's last-known UpdatedAt to
// trigger 409 Conflict if the row has been modified since.
type MapService interface {
	// Map CRUD.
	CreateMap(ctx context.Context, input CreateMapInput) (*Map, error)
	GetMap(ctx context.Context, id string) (*Map, error)
	UpdateMap(ctx context.Context, id string, input UpdateMapInput) error
	DeleteMap(ctx context.Context, id string, expectedUpdatedAt *time.Time) error
	ListMaps(ctx context.Context, campaignID string) ([]Map, error)
	SearchMaps(ctx context.Context, campaignID, query string) ([]map[string]string, error)

	// Map look. GetCampaignFrame returns the campaign-wide frame (the default
	// when the campaign has never chosen); SetCampaignFrame validates and
	// stores it; ResolveDisplay is a map's settings with every default filled,
	// including the campaign frame it follows.
	GetCampaignFrame(ctx context.Context, campaignID string) (string, error)
	SetCampaignFrame(ctx context.Context, campaignID, frame string) error
	ResolveDisplay(ctx context.Context, m *Map) (ResolvedDisplay, error)

	// Marker CRUD. UpdateMarker and DeleteMarker take canAuthorDmOnly (Owner
	// or a co-DM grant, campaigns.CampaignContext.CanAuthorDmOnly) so a
	// caller who cannot author dm_only content gets the same NotFound a
	// missing id would give when the stored marker is dm_only, regardless of
	// which fields the request touches.
	CreateMarker(ctx context.Context, input CreateMarkerInput) (*Marker, error)
	GetMarker(ctx context.Context, id string) (*Marker, error)
	UpdateMarker(ctx context.Context, id string, input UpdateMarkerInput, canAuthorDmOnly bool) error
	DeleteMarker(ctx context.Context, id string, expectedUpdatedAt *time.Time, canAuthorDmOnly bool, actorID string, role int) error
	// ListMarkers takes campaignID so non-owner results can be narrowed by
	// EntityVisibilityGate: the marker's own visibility is repo-filtered, but
	// a linked entity's visibility is a separate check the caller must supply.
	ListMarkers(ctx context.Context, campaignID, mapID string, role int, userID string) ([]Marker, error)
	// IsMarkerShadowed reports whether a viewer of this role must not receive
	// the marker because it lies under a shadow area. By-id reads use it so
	// they cannot reveal what ListMarkers withholds.
	IsMarkerShadowed(ctx context.Context, mk *Marker, role int) (bool, error)

	// ForViewer returns the map as a viewer of this (promoted visibility) role
	// may receive it. For anyone who is not owner/DM-equivalent and a map with a
	// shadow, the original image id is removed and the address of the player copy
	// (the picture with the shadows smudged into the pixels) is set instead.
	// Every handler that sends a map to a user passes it through here.
	ForViewer(ctx context.Context, m *Map, role int) (*Map, error)
	ForViewerList(ctx context.Context, ms []Map, role int) ([]Map, error)
	// PlayerImage renders (or reads from cache) the player copy of the map's
	// picture as JPEG. It errors rather than ever returning the original.
	PlayerImage(ctx context.Context, m *Map) ([]byte, error)
	// IsShadowedMapImage tells the media plugin whether a file is the picture of
	// a map that has a shadow (media.MapImageGuard); IsMapPicture whether it is
	// the picture of any map at all.
	IsShadowedMapImage(ctx context.Context, campaignID, mediaID string) (bool, error)
	IsMapPicture(ctx context.Context, campaignID, mediaID string) (bool, error)

	// Wiring.
	SetEventPublisher(pub MapEventPublisher)
	// SetShadowLookup is on the interface (not reached by type assertion) so a
	// wiring that stops matching fails to compile instead of silently showing
	// players every pin.
	SetShadowLookup(l ShadowLookup)
	// SetPlayerImageSource is on the interface for the same reason.
	SetPlayerImageSource(src MediaImageSource, cacheDir string)
}

// EntityVisibilityGate resolves which of a set of entity IDs a viewer (role +
// userID) may see, applying the entities plugin's own canonical visibility
// policy (default is_private, custom per-subject grants, tag grants). Wraps
// entities.EntityService.FilterViewableEntityIDs — the same seam armory,
// media, npcs, sessions and the relations widget use — so a marker naming a
// dm_only/private entity never leaks that entity's name/icon to a viewer who
// could not otherwise see it.
type EntityVisibilityGate interface {
	FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error)
}

// ShadowLookup returns the shadow areas of a map. Implemented by the drawing
// service, which owns drawings; the map service only applies the rule to pins.
type ShadowLookup interface {
	ShadowAreas(ctx context.Context, mapID string) ([]ShadowArea, error)
}

// mapService is the default MapService implementation.
type mapService struct {
	repo           MapRepository
	events         MapEventPublisher
	bindingCleaner BindingCleaner
	entityGate     EntityVisibilityGate
	shadows        ShadowLookup
	images         MediaImageSource
	imageCacheDir  string
	pictureCache   *pictureStatusCache
}

// BindingCleaner sweeps a deleted instance's widget bindings. Implemented by
// widgetbindings.Service; injected via SetBindingCleaner. Optional — nil means
// "no binding framework wired" (the render-time guard + Sweep are the backstop).
type BindingCleaner interface {
	OnInstanceDeleted(ctx context.Context, campaignID, widgetType, instanceID string) (int, error)
}

// NewMapService creates a MapService backed by the given repository.
func NewMapService(repo MapRepository) MapService {
	return &mapService{repo: repo, events: NoopMapEventPublisher{}, pictureCache: newPictureStatusCache()}
}

// SetBindingCleaner injects the widget-binding cleanup hook (wired at app
// startup). Reached via a type assertion in routes.go so the MapService
// interface stays unchanged.
func (s *mapService) SetBindingCleaner(c BindingCleaner) { s.bindingCleaner = c }

// SetEntityVisibilityGate injects the entity-visibility check used by
// ListMarkers (wired post-construction, like SetBindingCleaner, since
// entityService and mapsService are constructed in each other's dependency
// order in routes.go). Nil is a valid — if unwired — value: ListMarkers
// fails closed and blanks every entity-linked marker's name/icon rather than
// risk showing one nothing verified as viewable.
func (s *mapService) SetEntityVisibilityGate(g EntityVisibilityGate) { s.entityGate = g }

// SetShadowLookup injects the shadow source (wired post-construction because
// the drawing service is built after this one). Unwired means no shadows are
// known, which is correct only for tests and installs without drawings.
//
// A source that can announce its writes (the drawing service) is hooked so the
// picture cache is dropped on every shadow change; changing the source drops it
// too.
func (s *mapService) SetShadowLookup(l ShadowLookup) {
	s.shadows = l
	s.pictureCache.invalidate("")
	if n, ok := l.(shadowChangeNotifier); ok {
		n.SetShadowChangeHook(s.InvalidateMapPictures)
	}
}

// shadowAreasFor returns the map's shadows for a viewer subject to hiding, or
// nil when the viewer is exempt or nothing is wired.
func (s *mapService) shadowAreasFor(ctx context.Context, mapID string, role int) ([]ShadowArea, error) {
	if s.shadows == nil || !shadowHidingApplies(role) {
		return nil, nil
	}
	return s.shadows.ShadowAreas(ctx, mapID)
}

// IsMarkerShadowed implements MapService. On a lookup error it answers true
// alongside the error, so a caller that ignores the error still fails closed.
func (s *mapService) IsMarkerShadowed(ctx context.Context, mk *Marker, role int) (bool, error) {
	if mk == nil {
		return false, nil
	}
	areas, err := s.shadowAreasFor(ctx, mk.MapID, role)
	if err != nil {
		return true, err
	}
	return MarkerUnderShadow(areas, mk), nil
}

// SetEventPublisher sets the event publisher for real-time marker sync.
func (s *mapService) SetEventPublisher(pub MapEventPublisher) {
	s.events = pub
}

// campaignForMap resolves the campaign ID for a map (used for event publishing).
func (s *mapService) campaignForMap(ctx context.Context, mapID string) string {
	m, err := s.repo.GetMap(ctx, mapID)
	if err != nil || m == nil {
		return ""
	}
	return m.CampaignID
}

// CreateMap creates a new map for a campaign.
func (s *mapService) CreateMap(ctx context.Context, input CreateMapInput) (*Map, error) {
	if input.Name == "" {
		return nil, apperror.NewValidation("map name is required")
	}
	if input.CampaignID == "" {
		return nil, apperror.NewValidation("campaign ID is required")
	}

	m := &Map{
		ID:          generateID(),
		CampaignID:  input.CampaignID,
		Name:        input.Name,
		Description: input.Description,
		ImageID:     input.ImageID,
		ImageWidth:  input.ImageWidth,
		ImageHeight: input.ImageHeight,
	}
	if err := s.repo.CreateMap(ctx, m); err != nil {
		return nil, fmt.Errorf("create map: %w", err)
	}
	s.InvalidateMapPictures(m.CampaignID)
	return m, nil
}

// GetMap returns a map by ID, or a not-found error.
func (s *mapService) GetMap(ctx context.Context, id string) (*Map, error) {
	m, err := s.repo.GetMap(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get map: %w", err)
	}
	if m == nil {
		return nil, apperror.NewNotFound("map not found")
	}
	return m, nil
}

// UpdateMap modifies an existing map.
func (s *mapService) UpdateMap(ctx context.Context, id string, input UpdateMapInput) error {
	m, err := s.repo.GetMap(ctx, id)
	if err != nil {
		return fmt.Errorf("get map for update: %w", err)
	}
	if m == nil {
		return apperror.NewNotFound("map not found")
	}

	if err := concurrency.Check(m.UpdatedAt, input.ExpectedUpdatedAt, "map"); err != nil {
		return err
	}

	if input.Name == "" {
		return apperror.NewValidation("map name is required")
	}

	// Load-merge-write: `m` is the row as stored, so every merge below
	// defaults to the stored value; only a key the caller actually sent
	// can change anything.
	m.Name = input.Name
	m.Description = input.Description.Ptr(m.Description)
	m.ImageID = input.ImageID.Ptr(m.ImageID)
	m.ImageWidth = input.ImageWidth.Val(m.ImageWidth)
	m.ImageHeight = input.ImageHeight.Val(m.ImageHeight)
	// BackgroundColor tri-state per the input docstring: nil leaves
	// unchanged; pointer-to-empty clears the override; any other value
	// sets it. Maps the empty-string clear to a nil DB value so
	// `WHERE background_color IS NULL` queries continue to work.
	if input.BackgroundColor != nil {
		if *input.BackgroundColor == "" {
			m.BackgroundColor = nil
		} else {
			if !colorPattern.MatchString(*input.BackgroundColor) {
				return apperror.NewValidation("background_color must be a valid hex colour (e.g., #1f2937)")
			}
			s := *input.BackgroundColor
			m.BackgroundColor = &s
		}
	}
	// Display settings: absent preserves, null clears every group, an object
	// replaces the groups it names (MergeDisplaySettings).
	if input.DisplaySettings.Present() {
		if input.DisplaySettings.IsNull() {
			m.Display = nil
		} else {
			raw, _ := input.DisplaySettings.Get()
			next, err := MergeDisplaySettings(m.Display, raw)
			if err != nil {
				return err
			}
			m.Display = next
		}
	}

	if err := s.repo.UpdateMap(ctx, m); err != nil {
		return fmt.Errorf("update map: %w", err)
	}
	s.InvalidateMapPictures(m.CampaignID)
	return nil
}

// GetCampaignFrame returns the campaign-wide frame style, or the default when
// the campaign has not chosen or its stored value is no longer a known frame.
func (s *mapService) GetCampaignFrame(ctx context.Context, campaignID string) (string, error) {
	frame, err := s.repo.GetCampaignFrame(ctx, campaignID)
	if err != nil {
		return "", fmt.Errorf("get campaign frame: %w", err)
	}
	if !IsValidFrame(frame) {
		return DefaultFrame, nil
	}
	return frame, nil
}

// SetCampaignFrame stores the campaign-wide frame style.
func (s *mapService) SetCampaignFrame(ctx context.Context, campaignID, frame string) error {
	if campaignID == "" {
		return apperror.NewValidation("campaign ID is required")
	}
	if !IsValidFrame(frame) {
		return apperror.NewValidation("frame must be one of: atlas, arcane, old, modern, futuristic, gilded")
	}
	if err := s.repo.SetCampaignFrame(ctx, campaignID, frame); err != nil {
		return fmt.Errorf("set campaign frame: %w", err)
	}
	return nil
}

// ResolveDisplay returns the map's settings with defaults filled in.
func (s *mapService) ResolveDisplay(ctx context.Context, m *Map) (ResolvedDisplay, error) {
	campaignID := ""
	if m != nil {
		campaignID = m.CampaignID
	}
	frame, err := s.GetCampaignFrame(ctx, campaignID)
	if err != nil {
		return ResolveDisplay(m, DefaultFrame), err
	}
	return ResolveDisplay(m, frame), nil
}

// DeleteMap removes a map and its markers.
func (s *mapService) DeleteMap(ctx context.Context, id string, expectedUpdatedAt *time.Time) error {
	m, err := s.repo.GetMap(ctx, id)
	if err != nil {
		return fmt.Errorf("get map for delete: %w", err)
	}
	if m == nil {
		return apperror.NewNotFound("map not found")
	}
	if err := concurrency.Check(m.UpdatedAt, expectedUpdatedAt, "map"); err != nil {
		return err
	}
	if err := s.repo.DeleteMap(ctx, id); err != nil {
		return fmt.Errorf("delete map: %w", err)
	}
	s.InvalidateMapPictures(m.CampaignID)
	// Widget-binding delete hook: sweep this map's widget_bindings rows.
	// Best-effort — the render-time orphan guard + Sweep backstop it. The
	// legacy entity.map_id is independently SET-NULLed by the
	// fk_entities_map_id ON DELETE SET NULL constraint.
	if s.bindingCleaner != nil {
		_, _ = s.bindingCleaner.OnInstanceDeleted(ctx, m.CampaignID, WidgetTypeMap, id)
	}
	return nil
}

// ListMaps returns all maps for a campaign.
func (s *mapService) ListMaps(ctx context.Context, campaignID string) ([]Map, error) {
	maps, err := s.repo.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list maps: %w", err)
	}
	return maps, nil
}

// CreateMarker places a new marker on a map.
func (s *mapService) CreateMarker(ctx context.Context, input CreateMarkerInput) (*Marker, error) {
	if input.Name == "" {
		return nil, apperror.NewValidation("marker name is required")
	}
	if input.MapID == "" {
		return nil, apperror.NewValidation("map ID is required")
	}
	if input.X < 0 || input.X > 100 || input.Y < 0 || input.Y > 100 {
		return nil, apperror.NewValidation("marker coordinates must be 0-100")
	}
	if err := validatePinCategory(input.PinCategory); err != nil {
		return nil, err
	}
	if input.Visibility == "" {
		input.Visibility = "everyone"
	}
	icon, err := sanitize.ValidateIcon(input.Icon)
	if err != nil {
		return nil, err
	}
	if icon == "" {
		icon = "fa-map-pin"
	}
	input.Icon = icon
	if input.Color == "" {
		input.Color = "#3b82f6"
	}

	// Validate color to prevent XSS (it is rendered into HTML).
	if !colorPattern.MatchString(input.Color) {
		return nil, apperror.NewValidation("color must be a valid hex color (e.g., #3b82f6)")
	}

	mk := &Marker{
		ID:              generateID(),
		MapID:           input.MapID,
		Name:            input.Name,
		Description:     input.Description,
		X:               input.X,
		Y:               input.Y,
		Icon:            input.Icon,
		Color:           input.Color,
		PinCategory:     input.PinCategory,
		EntityID:        input.EntityID,
		Visibility:      input.Visibility,
		VisibilityRules: input.VisibilityRules,
		CreatedBy:       &input.CreatedBy,
		FoundryID:       input.FoundryID,
	}
	if err := s.repo.CreateMarker(ctx, mk); err != nil {
		return nil, fmt.Errorf("create marker: %w", err)
	}
	s.events.PublishMarkerEvent("created", s.campaignForMap(ctx, mk.MapID), mk)
	return mk, nil
}

// GetMarker returns a single marker by ID.
func (s *mapService) GetMarker(ctx context.Context, id string) (*Marker, error) {
	mk, err := s.repo.GetMarker(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get marker: %w", err)
	}
	if mk == nil {
		return nil, apperror.NewNotFound("marker not found")
	}
	return mk, nil
}

// UpdateMarker modifies an existing marker.
func (s *mapService) UpdateMarker(ctx context.Context, id string, input UpdateMarkerInput, canAuthorDmOnly bool) error {
	mk, err := s.repo.GetMarker(ctx, id)
	if err != nil {
		return fmt.Errorf("get marker for update: %w", err)
	}
	if mk == nil {
		return apperror.NewNotFound("marker not found")
	}
	// A stored dm_only marker is invisible to a caller who cannot author
	// dm_only content: answer the same NotFound a missing id would give,
	// whatever fields this update touches, so this can't be used to confirm
	// the marker exists or to change it without ever seeing it.
	if mk.Visibility == "dm_only" && !canAuthorDmOnly {
		return apperror.NewNotFound("marker not found")
	}

	if err := concurrency.Check(mk.UpdatedAt, input.ExpectedUpdatedAt, "marker"); err != nil {
		return err
	}

	// Load-merge-write: `mk` is the row as stored, so every merge below
	// defaults to the stored value; only a key the caller actually sent can
	// change it. Validators read the MERGED value, not the raw input — an
	// absent name is not an empty name.
	name := input.Name.Val(mk.Name)
	if name == "" {
		return apperror.NewValidation("marker name is required")
	}
	x, y := input.X.Val(mk.X), input.Y.Val(mk.Y)
	if x < 0 || x > 100 || y < 0 || y > 100 {
		return apperror.NewValidation("marker coordinates must be 0-100")
	}
	if pc, sent := input.PinCategory.Get(); sent {
		if err := validatePinCategory(&pc); err != nil {
			return err
		}
	}

	// Validate icon and color to prevent XSS (these are rendered into HTML).
	icon, err := sanitize.ValidateIcon(input.Icon.Val(mk.Icon))
	if err != nil {
		return err
	}
	if icon == "" {
		icon = "fa-map-pin"
	}
	color := input.Color.Val(mk.Color)
	if color != "" && !colorPattern.MatchString(color) {
		return apperror.NewValidation("color must be a valid hex color")
	}

	mk.Name = name
	mk.Description = input.Description.Ptr(mk.Description)
	mk.X = x
	mk.Y = y
	mk.Icon = icon
	mk.Color = color
	mk.PinCategory = input.PinCategory.Ptr(mk.PinCategory)
	mk.EntityID = input.EntityID.Ptr(mk.EntityID)
	mk.Visibility = input.Visibility.Val(mk.Visibility)
	mk.VisibilityRules = input.VisibilityRules.Ptr(mk.VisibilityRules)
	mk.FoundryID = input.FoundryID.Ptr(mk.FoundryID)

	if err := s.repo.UpdateMarker(ctx, mk); err != nil {
		return fmt.Errorf("update marker: %w", err)
	}
	s.events.PublishMarkerEvent("updated", s.campaignForMap(ctx, mk.MapID), mk)
	return nil
}

// canDeleteOwn is the shared delete rule for scribe-authored map content:
// Owners delete anything; a Scribe only what they created; lower roles nothing. An empty actor
// or a missing creator never matches, so rows without provenance stay
// owner-only.
func canDeleteOwn(role int, actorID string, createdBy *string) bool {
	if role >= permissions.RoleOwner {
		return true
	}
	if role < permissions.RoleScribe {
		return false
	}
	return actorID != "" && createdBy != nil && *createdBy == actorID
}

// DeleteMarker removes a marker. Owners may delete any marker; a lower role
// may delete only a marker it created. A marker with no recorded creator is
// owner-only so legacy rows fail closed.
func (s *mapService) DeleteMarker(ctx context.Context, id string, expectedUpdatedAt *time.Time, canAuthorDmOnly bool, actorID string, role int) error {
	mk, err := s.repo.GetMarker(ctx, id)
	if err != nil {
		return fmt.Errorf("get marker for delete: %w", err)
	}
	if mk == nil {
		return apperror.NewNotFound("marker not found")
	}
	// See UpdateMarker: a stored dm_only marker answers the same NotFound a
	// missing id would to a caller who cannot author dm_only content.
	if mk.Visibility == "dm_only" && !canAuthorDmOnly {
		return apperror.NewNotFound("marker not found")
	}
	if !canDeleteOwn(role, actorID, mk.CreatedBy) {
		return apperror.NewForbidden("you can only delete markers you created")
	}
	if err := concurrency.Check(mk.UpdatedAt, expectedUpdatedAt, "marker"); err != nil {
		return err
	}
	if err := s.repo.DeleteMarker(ctx, id); err != nil {
		return fmt.Errorf("delete marker: %w", err)
	}
	s.events.PublishMarkerEvent("deleted", s.campaignForMap(ctx, mk.MapID), mk)
	return nil
}

// ListMarkers returns all markers for a map, filtered by role and user. The
// repo already drops markers the viewer's own visibility/visibility_rules
// exclude; this layer additionally blanks EntityName/EntityIcon on any
// remaining marker whose linked entity the viewer isn't separately permitted
// to see, so an 'everyone' marker can never name a private entity.
func (s *mapService) ListMarkers(ctx context.Context, campaignID, mapID string, role int, userID string) ([]Marker, error) {
	markers, err := s.repo.ListMarkers(ctx, mapID, role, userID)
	if err != nil {
		return nil, fmt.Errorf("list markers: %w", err)
	}

	// Owners already see every marker unfiltered at the repo layer; the
	// entity link they see is the entity link that exists, same as the DM
	// view everywhere else in the app.
	if permissions.CanSeeDmOnly(role) {
		return markers, nil
	}

	// Pins under a shadow are dropped before anything else so their linked
	// entities are never even looked up for this viewer.
	areas, err := s.shadowAreasFor(ctx, mapID, role)
	if err != nil {
		return nil, fmt.Errorf("list shadow areas: %w", err)
	}
	markers = filterMarkersByShadow(areas, markers)

	entityIDs := make([]string, 0, len(markers))
	seen := make(map[string]bool, len(markers))
	for _, mk := range markers {
		if mk.EntityID == nil || *mk.EntityID == "" || seen[*mk.EntityID] {
			continue
		}
		seen[*mk.EntityID] = true
		entityIDs = append(entityIDs, *mk.EntityID)
	}
	if len(entityIDs) == 0 {
		return markers, nil
	}

	var viewable map[string]bool
	if s.entityGate == nil {
		// Misconfiguration, not a policy outcome: fail closed exactly like
		// media's checkMediaAccess does when its own gate is unwired.
		slog.Error("maps: entity visibility gate not configured; blanking all linked entity names",
			slog.String("campaign_id", campaignID), slog.String("map_id", mapID))
		viewable = map[string]bool{}
	} else {
		viewable, err = s.entityGate.FilterViewableEntityIDs(ctx, campaignID, entityIDs, role, userID)
		if err != nil {
			return nil, fmt.Errorf("filter viewable marker entities: %w", err)
		}
	}

	for i := range markers {
		mk := &markers[i]
		if mk.EntityID == nil || *mk.EntityID == "" {
			continue
		}
		if !viewable[*mk.EntityID] {
			// Blank the ID too, not just the name/icon: every consumer of
			// this struct (page HTML, the web JSON APIs, the sync API)
			// serializes EntityID verbatim, and a bare ID still tells the
			// viewer a specific hidden entity exists — the existence leak
			// ADR-055 rule 3 forbids, not just the name leak.
			mk.EntityID = nil
			mk.EntityName = ""
			mk.EntityIcon = ""
		}
	}
	return markers, nil
}

// SearchMaps returns maps matching a query as map results for the quick search system.
// Results are formatted to match the entity search JSON format.
func (s *mapService) SearchMaps(ctx context.Context, campaignID, query string) ([]map[string]string, error) {
	maps, err := s.repo.SearchMaps(ctx, campaignID, query)
	if err != nil {
		return nil, fmt.Errorf("search maps: %w", err)
	}

	results := make([]map[string]string, 0, len(maps))
	for _, m := range maps {
		results = append(results, map[string]string{
			"id":         m.ID,
			"name":       m.Name,
			"type_name":  "Map",
			"type_icon":  "fa-map",
			"type_color": "#10b981",
			"url":        fmt.Sprintf("/campaigns/%s/maps/%s", campaignID, m.ID),
		})
	}
	return results, nil
}
