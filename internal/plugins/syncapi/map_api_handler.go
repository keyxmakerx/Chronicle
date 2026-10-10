// Package syncapi — map_api_handler.go provides REST API v1 endpoints for
// map, drawing, token, layer, fog, and marker CRUD. External clients (Foundry
// VTT) use these endpoints to synchronize map data with Chronicle via API key
// auth.
package syncapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
)

// MapAPIHandler serves map-related REST API endpoints for external tools.
type MapAPIHandler struct {
	syncSvc     SyncAPIService
	mapSvc      maps.MapService
	drawingSvc  maps.DrawingService
	campaignSvc campaigns.CampaignService
}

// NewMapAPIHandler creates a new map API handler.
func NewMapAPIHandler(syncSvc SyncAPIService, mapSvc maps.MapService, drawingSvc maps.DrawingService, campaignSvc campaigns.CampaignService) *MapAPIHandler {
	return &MapAPIHandler{
		syncSvc:     syncSvc,
		mapSvc:      mapSvc,
		drawingSvc:  drawingSvc,
		campaignSvc: campaignSvc,
	}
}

// resolveRole returns the API key owner's role for visibility filtering.
func (h *MapAPIHandler) resolveRole(c echo.Context) int {
	key := GetAPIKey(c)
	if key == nil {
		return 0
	}
	member, err := h.campaignSvc.GetMember(c.Request().Context(), key.CampaignID, key.UserID)
	if err != nil {
		return 0
	}
	return int(member.Role)
}

// viewRole is the role the web applies when deciding what a viewer may see
// (campaigns.CampaignContext.VisibilityRole): the member role, promoted to
// owner for a co-DM grant. A failed grant lookup leaves the raw role, which only
// ever hides more. Writes keep using resolveRole.
func (h *MapAPIHandler) viewRole(c echo.Context) int {
	role := h.resolveRole(c)
	key := GetAPIKey(c)
	if key == nil || role >= int(campaigns.RoleOwner) {
		return role
	}
	if granted, err := h.campaignSvc.IsUserDmGranted(c.Request().Context(), key.CampaignID, key.UserID); err == nil && granted {
		return int(campaigns.RoleOwner)
	}
	return role
}

// requireOwnerRole enforces the same Owner-only floor internal/plugins/maps
// applies to GM-only map operations (fog of war, layer structure, and
// token deletion; marker and drawing deletion is Scribe+ with the service
// limiting a Scribe to its own items) on the web: RequirePermission(PermWrite)
// alone also admits a Scribe key, which the web route refuses for these
// actions. Fails closed — an unresolved role (resolveRole's 0) is refused
// same as any role below Owner.
func (h *MapAPIHandler) requireOwnerRole(c echo.Context) error {
	if campaigns.Role(h.resolveRole(c)) < campaigns.RoleOwner {
		return apperror.NewForbidden("owner role required for this map operation")
	}
	return nil
}

// canAuthorDmOnly reports whether the API key's user may author/update
// dm_only content — Owner role, or a co-DM grant — mirroring
// campaigns.CampaignContext.CanAuthorDmOnly for a caller that only has an
// API key, not a CampaignContext.
func (h *MapAPIHandler) canAuthorDmOnly(c echo.Context) bool {
	key := GetAPIKey(c)
	if key == nil {
		return false
	}
	if campaigns.Role(h.resolveRole(c)) >= campaigns.RoleOwner {
		return true
	}
	granted, err := h.campaignSvc.IsUserDmGranted(c.Request().Context(), key.CampaignID, key.UserID)
	return err == nil && granted
}

// ownerVisibilityRules keeps per-player visibility rules on a new marker only
// for an Owner, as the web route does; anyone else's are dropped.
func (h *MapAPIHandler) ownerVisibilityRules(c echo.Context, rules *string) *string {
	if campaigns.Role(h.resolveRole(c)) < campaigns.RoleOwner {
		return nil
	}
	return rules
}

// ownerVisibilityRulesPatch is ownerVisibilityRules for an update: a non-Owner's
// rules become absent, never null, so refusing the write can't erase the
// Owner's existing rules.
func (h *MapAPIHandler) ownerVisibilityRulesPatch(c echo.Context, rules patch.Field[string]) patch.Field[string] {
	if campaigns.Role(h.resolveRole(c)) < campaigns.RoleOwner {
		return patch.Absent[string]()
	}
	return rules
}

// requireMapInCampaign validates that the map belongs to the campaign in the URL.
func (h *MapAPIHandler) requireMapInCampaign(c echo.Context) (*maps.Map, error) {
	campaignID := c.Param("id")
	mapID := c.Param("mapID")
	ctx := c.Request().Context()

	m, err := h.mapSvc.GetMap(ctx, mapID)
	if err != nil {
		return nil, apperror.NewNotFound("map not found")
	}
	if m.CampaignID != campaignID {
		return nil, apperror.NewForbidden("map does not belong to this campaign")
	}
	return m, nil
}

// requireMarkerOnMap loads a marker and checks it sits on the map the URL
// names, which requireMapInCampaign has already tied to the key's campaign.
// The map service looks markers up by id alone, so without this a marker id
// from another campaign would be readable and writable through this route.
// A mismatch answers the same NotFound as a missing id.
func (h *MapAPIHandler) requireMarkerOnMap(c echo.Context, m *maps.Map) (*maps.Marker, error) {
	mk, err := h.mapSvc.GetMarker(c.Request().Context(), c.Param("markerID"))
	if err != nil {
		return nil, err
	}
	if mk.MapID != m.ID {
		return nil, apperror.NewNotFound("marker not found")
	}
	return mk, nil
}

// --- Map CRUD ---

// ListMaps returns all maps for a campaign.
// GET /api/v1/campaigns/:id/maps
func (h *MapAPIHandler) ListMaps(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	result, err := h.mapSvc.ListMaps(ctx, campaignID)
	if err != nil {
		slog.Error("api: list maps failed", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list maps"))
	}
	// The player-copy address is worked out from the stored maps, before
	// ForViewerList strips the original id from a below-owner key's copy.
	apiURLs := make([]string, len(result))
	for i := range result {
		if apiURLs[i], err = h.playerImageAPIURL(ctx, &result[i]); err != nil {
			slog.Error("api: prepare map pictures failed", slog.Any("error", err))
			return apperror.NewInternal(fmt.Errorf("failed to list maps"))
		}
	}
	// A key whose user is below owner gets the player copy's address, never the
	// original picture of a map that has a shadow.
	viewed, err := h.mapSvc.ForViewerList(ctx, result, h.viewRole(c))
	if err != nil {
		slog.Error("api: prepare map pictures failed", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list maps"))
	}
	// Copy so the service's maps are never mutated.
	out := make([]maps.Map, len(viewed))
	copy(out, viewed)
	for i := range out {
		out[i].PlayerImageAPIURL = apiURLs[i]
	}
	return c.JSON(http.StatusOK, out)
}

// playerImageAPIURL is the sync API address of the player copy of a stored map
// that has a shadow, or "". It is set for every key: the module syncs with the
// owner's key, which still reads the original, and this is the picture it must
// hand to players instead.
func (h *MapAPIHandler) playerImageAPIURL(ctx context.Context, m *maps.Map) (string, error) {
	v, err := h.mapSvc.PlayerImageVersion(ctx, m)
	if err != nil || v == "" {
		return "", err
	}
	return fmt.Sprintf("/api/v1/campaigns/%s/maps/%s/player-image?v=%s", m.CampaignID, m.ID, v), nil
}

// PlayerImage serves the map's picture with its shadowed areas smudged in, to
// any key that may read the map. Any failure to produce the copy is an error,
// never the original.
// GET /api/v1/campaigns/:id/maps/:mapID/player-image
func (h *MapAPIHandler) PlayerImage(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	data, err := h.mapSvc.PlayerImage(c.Request().Context(), m)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "private, max-age=86400")
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	return c.Blob(http.StatusOK, "image/jpeg", data)
}

// GetMap returns a single map with its markers.
// GET /api/v1/campaigns/:id/maps/:mapID
func (h *MapAPIHandler) GetMap(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}

	role := h.viewRole(c)
	ctx := c.Request().Context()

	// Resolve user ID from API key for per-player filtering.
	userID := ""
	if key := GetAPIKey(c); key != nil {
		userID = key.UserID
	}

	// Load markers for the map.
	markers, err := h.mapSvc.ListMarkers(ctx, m.CampaignID, m.ID, role, userID)
	if err != nil {
		slog.Error("api: list markers failed", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to load markers"))
	}
	apiURL, err := h.playerImageAPIURL(ctx, m)
	if err != nil {
		slog.Error("api: prepare map picture failed", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to load map"))
	}
	vm, err := h.mapSvc.ForViewer(ctx, m, role)
	if err != nil {
		slog.Error("api: prepare map picture failed", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to load map"))
	}
	// Set on the viewer's copy, so the stored map the service handed back is
	// never mutated.
	out := *vm
	out.Markers = markers
	out.PlayerImageAPIURL = apiURL

	return c.JSON(http.StatusOK, out)
}

// --- Drawing CRUD ---

// apiCreateDrawingRequest is the JSON body for creating a drawing via the API.
type apiCreateDrawingRequest struct {
	LayerID     *string         `json:"layer_id"`
	DrawingType string          `json:"drawing_type"`
	Points      json.RawMessage `json:"points"`
	StrokeColor string          `json:"stroke_color"`
	StrokeWidth float64         `json:"stroke_width"`
	FillColor   *string         `json:"fill_color"`
	FillAlpha   float64         `json:"fill_alpha"`
	TextContent *string         `json:"text_content"`
	FontSize    *int            `json:"font_size"`
	Rotation    float64         `json:"rotation"`
	Visibility  string          `json:"visibility"`
	FoundryID   *string         `json:"foundry_id"`
}

// ListDrawings returns all drawings for a map.
// GET /api/v1/campaigns/:id/maps/:mapID/drawings
func (h *MapAPIHandler) ListDrawings(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	role := h.viewRole(c)

	// Resolve user ID from API key for per-player visibility_rules
	// filtering (S1 — matches ListMarkers above; ListDrawings previously
	// ignored caller identity entirely, so a drawing's rules had no
	// effect over this API either).
	userID := ""
	if key := GetAPIKey(c); key != nil {
		userID = key.UserID
	}

	drawings, err := h.drawingSvc.ListDrawings(c.Request().Context(), m.ID, role, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("failed to list drawings"))
	}
	// A fogged hex layer's picture goes out without its file, as on the web.
	drawings, err = h.drawingSvc.WithholdImages(c.Request().Context(), m.ID, role, drawings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("failed to list drawings"))
	}
	return c.JSON(http.StatusOK, drawings)
}

// CreateDrawing creates a new drawing on a map.
// POST /api/v1/campaigns/:id/maps/:mapID/drawings
func (h *MapAPIHandler) CreateDrawing(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("api key required")
	}

	var req apiCreateDrawingRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	drawing, err := h.drawingSvc.CreateDrawing(c.Request().Context(), maps.CreateDrawingInput{
		MapID:       m.ID,
		LayerID:     req.LayerID,
		DrawingType: req.DrawingType,
		Points:      req.Points,
		StrokeColor: req.StrokeColor,
		StrokeWidth: req.StrokeWidth,
		FillColor:   req.FillColor,
		FillAlpha:   req.FillAlpha,
		TextContent: req.TextContent,
		FontSize:    req.FontSize,
		Rotation:    req.Rotation,
		Visibility:  req.Visibility,
		CreatedBy:   key.UserID,
		FoundryID:   req.FoundryID,
		CallerRole:  h.resolveRole(c),
		CallerIsDM:  h.canAuthorDmOnly(c),
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, drawing)
}

// apiUpdateDrawingRequest is the JSON body for updating a drawing.
// PARTIAL update: absent preserves, explicit null clears, a present value
// replaces (.ai/conventions.md, "Partial-Update Endpoints") — see maps.UpdateDrawingInput.
type apiUpdateDrawingRequest struct {
	Points            patch.Field[json.RawMessage] `json:"points"`
	StrokeColor       patch.Field[string]          `json:"stroke_color"`
	StrokeWidth       patch.Field[float64]         `json:"stroke_width"`
	FillColor         patch.Field[string]          `json:"fill_color"`
	FillAlpha         patch.Field[float64]         `json:"fill_alpha"`
	TextContent       patch.Field[string]          `json:"text_content"`
	FontSize          patch.Field[int]             `json:"font_size"`
	Rotation          patch.Field[float64]         `json:"rotation"`
	Visibility        patch.Field[string]          `json:"visibility"`
	ExpectedUpdatedAt *time.Time                   `json:"expected_updated_at"`
}

// UpdateDrawing updates an existing drawing.
// PUT /api/v1/campaigns/:id/maps/:mapID/drawings/:drawingID
func (h *MapAPIHandler) UpdateDrawing(c echo.Context) error {
	if _, err := h.requireMapInCampaign(c); err != nil {
		return err
	}
	drawingID := c.Param("drawingID")

	var req apiUpdateDrawingRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	err := h.drawingSvc.UpdateDrawing(c.Request().Context(), drawingID, c.Param("mapID"), h.resolveRole(c), h.canAuthorDmOnly(c), maps.UpdateDrawingInput{
		Points:            req.Points,
		StrokeColor:       req.StrokeColor,
		StrokeWidth:       req.StrokeWidth,
		FillColor:         req.FillColor,
		FillAlpha:         req.FillAlpha,
		TextContent:       req.TextContent,
		FontSize:          req.FontSize,
		Rotation:          req.Rotation,
		Visibility:        req.Visibility,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt,
	})
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteDrawing removes a drawing.
// DELETE /api/v1/campaigns/:id/maps/:mapID/drawings/:drawingID
func (h *MapAPIHandler) DeleteDrawing(c echo.Context) error {
	if _, err := h.requireMapInCampaign(c); err != nil {
		return err
	}
	// Scribe keys may delete only drawings their user created; the service
	// enforces that from the key owner's resolved role and user id.
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewForbidden("api key required")
	}
	if campaigns.Role(h.resolveRole(c)) < campaigns.RoleScribe {
		return apperror.NewForbidden("scribe role required to delete drawings")
	}
	if err := h.drawingSvc.DeleteDrawing(c.Request().Context(), c.Param("drawingID"), c.Param("mapID"), maps.ParseExpectedUpdatedAt(c), key.UserID, h.resolveRole(c), h.canAuthorDmOnly(c)); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Token CRUD ---

// apiCreateTokenRequest is the JSON body for creating a token via the API.
type apiCreateTokenRequest struct {
	LayerID        *string         `json:"layer_id"`
	EntityID       *string         `json:"entity_id"`
	Name           string          `json:"name"`
	ImagePath      *string         `json:"image_path"`
	X              float64         `json:"x"`
	Y              float64         `json:"y"`
	Width          float64         `json:"width"`
	Height         float64         `json:"height"`
	Rotation       float64         `json:"rotation"`
	Scale          float64         `json:"scale"`
	IsHidden       bool            `json:"is_hidden"`
	IsLocked       bool            `json:"is_locked"`
	Bar1Value      *int            `json:"bar1_value"`
	Bar1Max        *int            `json:"bar1_max"`
	Bar2Value      *int            `json:"bar2_value"`
	Bar2Max        *int            `json:"bar2_max"`
	AuraRadius     *float64        `json:"aura_radius"`
	AuraColor      *string         `json:"aura_color"`
	LightRadius    *float64        `json:"light_radius"`
	LightDimRadius *float64        `json:"light_dim_radius"`
	LightColor     *string         `json:"light_color"`
	VisionEnabled  bool            `json:"vision_enabled"`
	VisionRange    *float64        `json:"vision_range"`
	Elevation      int             `json:"elevation"`
	StatusEffects  json.RawMessage `json:"status_effects"`
	Flags          json.RawMessage `json:"flags"`
	FoundryID      *string         `json:"foundry_id"`
}

// ListTokens returns all tokens for a map.
// GET /api/v1/campaigns/:id/maps/:mapID/tokens
func (h *MapAPIHandler) ListTokens(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	role := h.resolveRole(c)
	tokens, err := h.drawingSvc.ListTokens(c.Request().Context(), m.ID, role)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("failed to list tokens"))
	}
	return c.JSON(http.StatusOK, tokens)
}

// CreateToken places a new token on a map.
// POST /api/v1/campaigns/:id/maps/:mapID/tokens
func (h *MapAPIHandler) CreateToken(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("api key required")
	}

	var req apiCreateTokenRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	token, err := h.drawingSvc.CreateToken(c.Request().Context(), maps.CreateTokenInput{
		MapID:          m.ID,
		LayerID:        req.LayerID,
		EntityID:       req.EntityID,
		Name:           req.Name,
		ImagePath:      req.ImagePath,
		X:              req.X,
		Y:              req.Y,
		Width:          req.Width,
		Height:         req.Height,
		Rotation:       req.Rotation,
		Scale:          req.Scale,
		IsHidden:       req.IsHidden,
		IsLocked:       req.IsLocked,
		Bar1Value:      req.Bar1Value,
		Bar1Max:        req.Bar1Max,
		Bar2Value:      req.Bar2Value,
		Bar2Max:        req.Bar2Max,
		AuraRadius:     req.AuraRadius,
		AuraColor:      req.AuraColor,
		LightRadius:    req.LightRadius,
		LightDimRadius: req.LightDimRadius,
		LightColor:     req.LightColor,
		VisionEnabled:  req.VisionEnabled,
		VisionRange:    req.VisionRange,
		Elevation:      req.Elevation,
		StatusEffects:  req.StatusEffects,
		Flags:          req.Flags,
		CreatedBy:      key.UserID,
		FoundryID:      req.FoundryID,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, token)
}

// apiUpdateTokenRequest is the JSON body for updating a token.
// PARTIAL update: absent preserves, explicit null clears, a present value
// replaces (.ai/conventions.md, "Partial-Update Endpoints") — see maps.UpdateTokenInput. This is the route a
// Foundry-side token drag hits, so an {x, y}-only push must not zero
// IsHidden, IsLocked, HP bars or aura/light/vision fields.
type apiUpdateTokenRequest struct {
	Name              string                       `json:"name"`
	ImagePath         patch.Field[string]          `json:"image_path"`
	X                 patch.Field[float64]         `json:"x"`
	Y                 patch.Field[float64]         `json:"y"`
	Width             patch.Field[float64]         `json:"width"`
	Height            patch.Field[float64]         `json:"height"`
	Rotation          patch.Field[float64]         `json:"rotation"`
	Scale             patch.Field[float64]         `json:"scale"`
	IsHidden          patch.Field[bool]            `json:"is_hidden"`
	IsLocked          patch.Field[bool]            `json:"is_locked"`
	Bar1Value         patch.Field[int]             `json:"bar1_value"`
	Bar1Max           patch.Field[int]             `json:"bar1_max"`
	Bar2Value         patch.Field[int]             `json:"bar2_value"`
	Bar2Max           patch.Field[int]             `json:"bar2_max"`
	AuraRadius        patch.Field[float64]         `json:"aura_radius"`
	AuraColor         patch.Field[string]          `json:"aura_color"`
	LightRadius       patch.Field[float64]         `json:"light_radius"`
	LightDimRadius    patch.Field[float64]         `json:"light_dim_radius"`
	LightColor        patch.Field[string]          `json:"light_color"`
	VisionEnabled     patch.Field[bool]            `json:"vision_enabled"`
	VisionRange       patch.Field[float64]         `json:"vision_range"`
	Elevation         patch.Field[int]             `json:"elevation"`
	StatusEffects     patch.Field[json.RawMessage] `json:"status_effects"`
	Flags             patch.Field[json.RawMessage] `json:"flags"`
	ExpectedUpdatedAt *time.Time                   `json:"expected_updated_at"`
}

// UpdateToken updates an existing token.
// PUT /api/v1/campaigns/:id/maps/:mapID/tokens/:tokenID
func (h *MapAPIHandler) UpdateToken(c echo.Context) error {
	if _, err := h.requireMapInCampaign(c); err != nil {
		return err
	}
	tokenID := c.Param("tokenID")

	var req apiUpdateTokenRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	err := h.drawingSvc.UpdateToken(c.Request().Context(), tokenID, c.Param("mapID"), h.canAuthorDmOnly(c), maps.UpdateTokenInput{
		Name:              req.Name,
		ImagePath:         req.ImagePath,
		X:                 req.X,
		Y:                 req.Y,
		Width:             req.Width,
		Height:            req.Height,
		Rotation:          req.Rotation,
		Scale:             req.Scale,
		IsHidden:          req.IsHidden,
		IsLocked:          req.IsLocked,
		Bar1Value:         req.Bar1Value,
		Bar1Max:           req.Bar1Max,
		Bar2Value:         req.Bar2Value,
		Bar2Max:           req.Bar2Max,
		AuraRadius:        req.AuraRadius,
		AuraColor:         req.AuraColor,
		LightRadius:       req.LightRadius,
		LightDimRadius:    req.LightDimRadius,
		LightColor:        req.LightColor,
		VisionEnabled:     req.VisionEnabled,
		VisionRange:       req.VisionRange,
		Elevation:         req.Elevation,
		StatusEffects:     req.StatusEffects,
		Flags:             req.Flags,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt,
	})
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// UpdateTokenPosition updates only the position (optimized for drag sync).
// PATCH /api/v1/campaigns/:id/maps/:mapID/tokens/:tokenID/position
func (h *MapAPIHandler) UpdateTokenPosition(c echo.Context) error {
	if _, err := h.requireMapInCampaign(c); err != nil {
		return err
	}
	tokenID := c.Param("tokenID")

	var req maps.UpdateTokenPositionInput
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	err := h.drawingSvc.UpdateTokenPosition(c.Request().Context(), tokenID, c.Param("mapID"), h.canAuthorDmOnly(c), req)
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteToken removes a token.
// DELETE /api/v1/campaigns/:id/maps/:mapID/tokens/:tokenID
func (h *MapAPIHandler) DeleteToken(c echo.Context) error {
	if _, err := h.requireMapInCampaign(c); err != nil {
		return err
	}
	if err := h.requireOwnerRole(c); err != nil {
		return err
	}
	if err := h.drawingSvc.DeleteToken(c.Request().Context(), c.Param("tokenID"), c.Param("mapID"), maps.ParseExpectedUpdatedAt(c)); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Layer CRUD ---

// apiCreateLayerRequest is the JSON body for creating a layer.
type apiCreateLayerRequest struct {
	Name      string  `json:"name"`
	LayerType string  `json:"layer_type"`
	SortOrder int     `json:"sort_order"`
	IsVisible bool    `json:"is_visible"`
	Opacity   float64 `json:"opacity"`
	IsLocked  bool    `json:"is_locked"`
}

// ListLayers returns all layers for a map.
// GET /api/v1/campaigns/:id/maps/:mapID/layers
func (h *MapAPIHandler) ListLayers(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	layers, err := h.drawingSvc.ListLayers(c.Request().Context(), m.ID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("failed to list layers"))
	}
	return c.JSON(http.StatusOK, layers)
}

// CreateLayer creates a new layer on a map.
// POST /api/v1/campaigns/:id/maps/:mapID/layers
func (h *MapAPIHandler) CreateLayer(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	if err := h.requireOwnerRole(c); err != nil {
		return err
	}

	var req apiCreateLayerRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	layer, err := h.drawingSvc.CreateLayer(c.Request().Context(), maps.CreateLayerInput{
		MapID:     m.ID,
		Name:      req.Name,
		LayerType: req.LayerType,
		SortOrder: req.SortOrder,
		IsVisible: req.IsVisible,
		Opacity:   req.Opacity,
		IsLocked:  req.IsLocked,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, layer)
}

// apiUpdateLayerRequest is the JSON body for updating a layer.
// PARTIAL update: absent preserves, explicit null clears, a present value
// replaces (.ai/conventions.md, "Partial-Update Endpoints") — see maps.UpdateLayerInput.
type apiUpdateLayerRequest struct {
	Name              string               `json:"name"`
	SortOrder         patch.Field[int]     `json:"sort_order"`
	IsVisible         patch.Field[bool]    `json:"is_visible"`
	Opacity           patch.Field[float64] `json:"opacity"`
	IsLocked          patch.Field[bool]    `json:"is_locked"`
	ExpectedUpdatedAt *time.Time           `json:"expected_updated_at"`
}

// UpdateLayer updates an existing layer.
// PUT /api/v1/campaigns/:id/maps/:mapID/layers/:layerID
func (h *MapAPIHandler) UpdateLayer(c echo.Context) error {
	if _, err := h.requireMapInCampaign(c); err != nil {
		return err
	}
	if err := h.requireOwnerRole(c); err != nil {
		return err
	}
	layerID := c.Param("layerID")

	var req apiUpdateLayerRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	err := h.drawingSvc.UpdateLayer(c.Request().Context(), layerID, c.Param("mapID"), maps.UpdateLayerInput{
		Name:              req.Name,
		SortOrder:         req.SortOrder,
		IsVisible:         req.IsVisible,
		Opacity:           req.Opacity,
		IsLocked:          req.IsLocked,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt,
	})
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteLayer removes a layer.
// DELETE /api/v1/campaigns/:id/maps/:mapID/layers/:layerID
func (h *MapAPIHandler) DeleteLayer(c echo.Context) error {
	if _, err := h.requireMapInCampaign(c); err != nil {
		return err
	}
	if err := h.requireOwnerRole(c); err != nil {
		return err
	}
	if err := h.drawingSvc.DeleteLayer(c.Request().Context(), c.Param("layerID"), c.Param("mapID"), maps.ParseExpectedUpdatedAt(c)); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Fog of War ---

// apiCreateFogRequest is the JSON body for creating a fog region.
type apiCreateFogRequest struct {
	Points     json.RawMessage `json:"points"`
	IsExplored bool            `json:"is_explored"`
}

// ListFog returns all fog regions for a map.
// GET /api/v1/campaigns/:id/maps/:mapID/fog
func (h *MapAPIHandler) ListFog(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	if err := h.requireOwnerRole(c); err != nil {
		return err
	}
	fog, err := h.drawingSvc.ListFog(c.Request().Context(), m.ID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("failed to list fog"))
	}
	return c.JSON(http.StatusOK, fog)
}

// CreateFog creates a fog region on a map.
// POST /api/v1/campaigns/:id/maps/:mapID/fog
func (h *MapAPIHandler) CreateFog(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	if err := h.requireOwnerRole(c); err != nil {
		return err
	}

	var req apiCreateFogRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	fog, err := h.drawingSvc.CreateFog(c.Request().Context(), maps.CreateFogInput{
		MapID:      m.ID,
		Points:     req.Points,
		IsExplored: req.IsExplored,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, fog)
}

// DeleteFog removes a fog region.
// DELETE /api/v1/campaigns/:id/maps/:mapID/fog/:fogID
func (h *MapAPIHandler) DeleteFog(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	if err := h.requireOwnerRole(c); err != nil {
		return err
	}
	// Pass the campaign-verified map ID so the service rejects a fog id that
	// belongs to another map (SEC-IDOR-4).
	if err := h.drawingSvc.DeleteFog(c.Request().Context(), c.Param("fogID"), m.ID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ResetFog removes all fog regions for a map.
// DELETE /api/v1/campaigns/:id/maps/:mapID/fog
func (h *MapAPIHandler) ResetFog(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	if err := h.requireOwnerRole(c); err != nil {
		return err
	}
	if err := h.drawingSvc.ResetFog(c.Request().Context(), m.ID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Marker CRUD ---

// apiCreateMarkerRequest is the JSON body for creating a marker via the API.
type apiCreateMarkerRequest struct {
	Name            string  `json:"name"`
	Description     *string `json:"description"`
	X               float64 `json:"x"`
	Y               float64 `json:"y"`
	Icon            string  `json:"icon"`
	Color           string  `json:"color"`
	PinCategory     *string `json:"pin_category"`
	EntityID        *string `json:"entity_id"`
	Visibility      string  `json:"visibility"`
	VisibilityRules *string `json:"visibility_rules"`
	FoundryID       *string `json:"foundry_id"`
}

// apiUpdateMarkerRequest is the JSON body for updating a marker.
// PARTIAL update: absent preserves, explicit null clears, a present value
// replaces. foundry_id keeps its clearability here — this is the surface
// that owns the pairing, and an explicit null still unpairs — while the
// Chronicle web form, which never sends the key, can no longer NULL it by
// omission.
type apiUpdateMarkerRequest struct {
	Name              patch.Field[string]  `json:"name"`
	Description       patch.Field[string]  `json:"description"`
	X                 patch.Field[float64] `json:"x"`
	Y                 patch.Field[float64] `json:"y"`
	Icon              patch.Field[string]  `json:"icon"`
	Color             patch.Field[string]  `json:"color"`
	PinCategory       patch.Field[string]  `json:"pin_category"`
	EntityID          patch.Field[string]  `json:"entity_id"`
	Visibility        patch.Field[string]  `json:"visibility"`
	VisibilityRules   patch.Field[string]  `json:"visibility_rules"`
	FoundryID         patch.Field[string]  `json:"foundry_id"`
	ExpectedUpdatedAt *time.Time           `json:"expected_updated_at"`
}

// ListMarkers returns all markers for a map, filtered by the caller's role.
// GET /api/v1/campaigns/:id/maps/:mapID/markers
func (h *MapAPIHandler) ListMarkers(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	role := h.viewRole(c)

	userID := ""
	if key := GetAPIKey(c); key != nil {
		userID = key.UserID
	}

	markers, err := h.mapSvc.ListMarkers(c.Request().Context(), m.CampaignID, m.ID, role, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("failed to list markers"))
	}
	return c.JSON(http.StatusOK, markers)
}

// GetMarker returns a single marker.
// GET /api/v1/campaigns/:id/maps/:mapID/markers/:markerID
func (h *MapAPIHandler) GetMarker(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	marker, err := h.requireMarkerOnMap(c, m)
	if err != nil {
		return err
	}
	// Same rule as the list: a dm_only marker, or one whose visibility rules
	// leave this user out, answers NotFound as a missing marker would.
	var userID string
	if key := GetAPIKey(c); key != nil {
		userID = key.UserID
	}
	if !maps.MarkerVisibleTo(marker, h.viewRole(c), userID) {
		return apperror.NewNotFound("marker not found")
	}
	// A pin under a shadow area answers NotFound to the roles the list hides
	// it from, using the same role the list endpoint resolves.
	shadowed, err := h.mapSvc.IsMarkerShadowed(c.Request().Context(), marker, h.viewRole(c))
	if err != nil {
		return err
	}
	if shadowed {
		return apperror.NewNotFound("marker not found")
	}
	return c.JSON(http.StatusOK, marker)
}

// CreateMarker places a new marker on a map.
// POST /api/v1/campaigns/:id/maps/:mapID/markers
func (h *MapAPIHandler) CreateMarker(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("api key required")
	}

	var req apiCreateMarkerRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	marker, err := h.mapSvc.CreateMarker(c.Request().Context(), maps.CreateMarkerInput{
		MapID:           m.ID,
		Name:            req.Name,
		Description:     req.Description,
		X:               req.X,
		Y:               req.Y,
		Icon:            req.Icon,
		Color:           req.Color,
		PinCategory:     req.PinCategory,
		EntityID:        req.EntityID,
		Visibility:      req.Visibility,
		VisibilityRules: h.ownerVisibilityRules(c, req.VisibilityRules),
		CreatedBy:       key.UserID,
		FoundryID:       req.FoundryID,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, marker)
}

// UpdateMarker updates an existing marker.
// PUT /api/v1/campaigns/:id/maps/:mapID/markers/:markerID
func (h *MapAPIHandler) UpdateMarker(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	if _, err := h.requireMarkerOnMap(c, m); err != nil {
		return err
	}
	markerID := c.Param("markerID")

	var req apiUpdateMarkerRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	err = h.mapSvc.UpdateMarker(c.Request().Context(), markerID, maps.UpdateMarkerInput{
		Name:              req.Name,
		Description:       req.Description,
		X:                 req.X,
		Y:                 req.Y,
		Icon:              req.Icon,
		Color:             req.Color,
		PinCategory:       req.PinCategory,
		EntityID:          req.EntityID,
		Visibility:        req.Visibility,
		VisibilityRules:   h.ownerVisibilityRulesPatch(c, req.VisibilityRules),
		FoundryID:         req.FoundryID,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt,
	}, h.canAuthorDmOnly(c))
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteMarker removes a marker.
// DELETE /api/v1/campaigns/:id/maps/:mapID/markers/:markerID
func (h *MapAPIHandler) DeleteMarker(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}
	// Scribe keys may delete only markers their user created; the service
	// enforces that from the key owner's resolved role and user id.
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewForbidden("api key required")
	}
	if campaigns.Role(h.resolveRole(c)) < campaigns.RoleScribe {
		return apperror.NewForbidden("scribe role required to delete markers")
	}
	if _, err := h.requireMarkerOnMap(c, m); err != nil {
		return err
	}
	if err := h.mapSvc.DeleteMarker(c.Request().Context(), c.Param("markerID"), maps.ParseExpectedUpdatedAt(c), h.canAuthorDmOnly(c), key.UserID, h.resolveRole(c)); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
