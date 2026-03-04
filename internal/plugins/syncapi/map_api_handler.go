// Package syncapi — map_api_handler.go provides REST API v1 endpoints for
// map, drawing, token, and layer CRUD. External clients (Foundry VTT) use
// these endpoints to synchronize map data with Chronicle via API key auth.
package syncapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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
	return c.JSON(http.StatusOK, result)
}

// GetMap returns a single map with its markers.
// GET /api/v1/campaigns/:id/maps/:mapID
func (h *MapAPIHandler) GetMap(c echo.Context) error {
	m, err := h.requireMapInCampaign(c)
	if err != nil {
		return err
	}

	role := h.resolveRole(c)
	ctx := c.Request().Context()

	// Load markers for the map.
	markers, err := h.mapSvc.ListMarkers(ctx, m.ID, role)
	if err != nil {
		slog.Error("api: list markers failed", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to load markers"))
	}
	m.Markers = markers

	return c.JSON(http.StatusOK, m)
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
	role := h.resolveRole(c)
	drawings, err := h.drawingSvc.ListDrawings(c.Request().Context(), m.ID, role)
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
	})
	if err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}
	return c.JSON(http.StatusCreated, drawing)
}

// apiUpdateDrawingRequest is the JSON body for updating a drawing.
type apiUpdateDrawingRequest struct {
	Points      json.RawMessage `json:"points"`
	StrokeColor string          `json:"stroke_color"`
	StrokeWidth float64         `json:"stroke_width"`
	FillColor   *string         `json:"fill_color"`
	FillAlpha   float64         `json:"fill_alpha"`
	TextContent *string         `json:"text_content"`
	FontSize    *int            `json:"font_size"`
	Rotation    float64         `json:"rotation"`
	Visibility  string          `json:"visibility"`
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

	err := h.drawingSvc.UpdateDrawing(c.Request().Context(), drawingID, maps.UpdateDrawingInput{
		Points:      req.Points,
		StrokeColor: req.StrokeColor,
		StrokeWidth: req.StrokeWidth,
		FillColor:   req.FillColor,
		FillAlpha:   req.FillAlpha,
		TextContent: req.TextContent,
		FontSize:    req.FontSize,
		Rotation:    req.Rotation,
		Visibility:  req.Visibility,
	})
	if err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteDrawing removes a drawing.
// DELETE /api/v1/campaigns/:id/maps/:mapID/drawings/:drawingID
func (h *MapAPIHandler) DeleteDrawing(c echo.Context) error {
	if _, err := h.requireMapInCampaign(c); err != nil {
		return err
	}
	if err := h.drawingSvc.DeleteDrawing(c.Request().Context(), c.Param("drawingID")); err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
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
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list tokens")
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
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}
	return c.JSON(http.StatusCreated, token)
}

// apiUpdateTokenRequest is the JSON body for updating a token.
type apiUpdateTokenRequest struct {
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

	err := h.drawingSvc.UpdateToken(c.Request().Context(), tokenID, maps.UpdateTokenInput{
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
	})
	if err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}
	return c.NoContent(http.StatusNoContent)
}

// apiUpdateTokenPositionRequest is the JSON body for moving a token.
type apiUpdateTokenPositionRequest struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// UpdateTokenPosition updates only the position (optimized for drag sync).
// PATCH /api/v1/campaigns/:id/maps/:mapID/tokens/:tokenID/position
func (h *MapAPIHandler) UpdateTokenPosition(c echo.Context) error {
	if _, err := h.requireMapInCampaign(c); err != nil {
		return err
	}
	tokenID := c.Param("tokenID")

	var req apiUpdateTokenPositionRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	err := h.drawingSvc.UpdateTokenPosition(c.Request().Context(), tokenID, maps.UpdateTokenPositionInput{
		X: req.X,
		Y: req.Y,
	})
	if err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteToken removes a token.
// DELETE /api/v1/campaigns/:id/maps/:mapID/tokens/:tokenID
func (h *MapAPIHandler) DeleteToken(c echo.Context) error {
	if _, err := h.requireMapInCampaign(c); err != nil {
		return err
	}
	if err := h.drawingSvc.DeleteToken(c.Request().Context(), c.Param("tokenID")); err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
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
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list layers")
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
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}
	return c.JSON(http.StatusCreated, layer)
}

// apiUpdateLayerRequest is the JSON body for updating a layer.
type apiUpdateLayerRequest struct {
	Name      string  `json:"name"`
	SortOrder int     `json:"sort_order"`
	IsVisible bool    `json:"is_visible"`
	Opacity   float64 `json:"opacity"`
	IsLocked  bool    `json:"is_locked"`
}

// UpdateLayer updates an existing layer.
// PUT /api/v1/campaigns/:id/maps/:mapID/layers/:layerID
func (h *MapAPIHandler) UpdateLayer(c echo.Context) error {
	if _, err := h.requireMapInCampaign(c); err != nil {
		return err
	}
	layerID := c.Param("layerID")

	var req apiUpdateLayerRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	err := h.drawingSvc.UpdateLayer(c.Request().Context(), layerID, maps.UpdateLayerInput{
		Name:      req.Name,
		SortOrder: req.SortOrder,
		IsVisible: req.IsVisible,
		Opacity:   req.Opacity,
		IsLocked:  req.IsLocked,
	})
	if err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteLayer removes a layer.
// DELETE /api/v1/campaigns/:id/maps/:mapID/layers/:layerID
func (h *MapAPIHandler) DeleteLayer(c echo.Context) error {
	if _, err := h.requireMapInCampaign(c); err != nil {
		return err
	}
	if err := h.drawingSvc.DeleteLayer(c.Request().Context(), c.Param("layerID")); err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
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
	fog, err := h.drawingSvc.ListFog(c.Request().Context(), m.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list fog")
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
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}
	return c.JSON(http.StatusCreated, fog)
}

// DeleteFog removes a fog region.
// DELETE /api/v1/campaigns/:id/maps/:mapID/fog/:fogID
func (h *MapAPIHandler) DeleteFog(c echo.Context) error {
	if _, err := h.requireMapInCampaign(c); err != nil {
		return err
	}
	if err := h.drawingSvc.DeleteFog(c.Request().Context(), c.Param("fogID")); err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
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
	if err := h.drawingSvc.ResetFog(c.Request().Context(), m.ID); err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}
	return c.NoContent(http.StatusNoContent)
}
