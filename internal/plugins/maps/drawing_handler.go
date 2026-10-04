package maps

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// DrawingHandler processes HTTP requests for drawings, tokens, layers, and fog.
// All endpoints are scoped to a campaign and require the map to belong to that campaign.
type DrawingHandler struct {
	mapSvc     MapService
	drawingSvc DrawingService
}

// NewDrawingHandler creates a new DrawingHandler.
func NewDrawingHandler(mapSvc MapService, drawingSvc DrawingService) *DrawingHandler {
	return &DrawingHandler{mapSvc: mapSvc, drawingSvc: drawingSvc}
}

// requireMapOwnership verifies the map belongs to the campaign. Returns 404 for
// cross-campaign IDOR attempts.
func (h *DrawingHandler) requireMapOwnership(c echo.Context, mapID, campaignID string) error {
	m, err := h.mapSvc.GetMap(c.Request().Context(), mapID)
	if err != nil {
		return err
	}
	if m.CampaignID != campaignID {
		return apperror.NewNotFound("map not found")
	}
	return nil
}

// getUserID extracts the user ID from the session context.
func getUserID(c echo.Context) string {
	if session := c.Get("session"); session != nil {
		if s, ok := session.(interface{ GetUserID() string }); ok {
			return s.GetUserID()
		}
	}
	return ""
}

// --- Drawing Endpoints ---

// drawingResponse is a drawing as sent to a viewer. ImageURL is the signed
// media URL of a picture, minted for THIS viewer (ADR-058): an unsigned
// /media/<id> is refused for an anonymous visitor of a public campaign and for
// a file that a hidden page also uses. It is added only to drawings the viewer
// was already allowed to receive, so a hidden picture never gets one.
type drawingResponse struct {
	Drawing
	ImageURL string `json:"image_url,omitempty"`
}

func drawingResponseFor(ctx context.Context, d Drawing) drawingResponse {
	r := drawingResponse{Drawing: d}
	if d.DrawingType == DrawingTypeImage && d.ImageID != nil && *d.ImageID != "" {
		r.ImageURL = layouts.MediaURL(ctx, *d.ImageID)
	}
	return r
}

// ListDrawings returns all drawings for a map, filtered by the user's role.
// GET /api/v1/campaigns/:id/maps/:mid/drawings
func (h *DrawingHandler) ListDrawings(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	role := cc.VisibilityRole()
	// userID for per-player visibility_rules filtering (S1) — matches how
	// the marker list endpoint resolves it (handler.go's ListMarkers).
	drawings, err := h.drawingSvc.ListDrawings(c.Request().Context(), mapID, role, getUserID(c))
	if err != nil {
		return err
	}
	out := make([]drawingResponse, len(drawings))
	for i, d := range drawings {
		out[i] = drawingResponseFor(c.Request().Context(), d)
	}
	return c.JSON(http.StatusOK, out)
}

// CreateDrawing creates a new drawing on a map.
// POST /api/v1/campaigns/:id/maps/:mid/drawings
func (h *DrawingHandler) CreateDrawing(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
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
		ImageID     *string         `json:"image_id"`
		Crop        json.RawMessage `json:"crop"`
		SortOrder   int             `json:"sort_order"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	d, err := h.drawingSvc.CreateDrawing(c.Request().Context(), CreateDrawingInput{
		MapID:       mapID,
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
		CreatedBy:   getUserID(c),
		FoundryID:   req.FoundryID,
		ImageID:     req.ImageID,
		Crop:        req.Crop,
		SortOrder:   req.SortOrder,
		CallerRole:  int(cc.MemberRole),
		CallerIsDM:  cc.CanAuthorDmOnly(),
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, drawingResponseFor(c.Request().Context(), *d))
}

// GetDrawing returns a single drawing by ID.
// GET /api/v1/campaigns/:id/maps/:mid/drawings/:did
func (h *DrawingHandler) GetDrawing(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	d, err := h.drawingSvc.GetDrawing(c.Request().Context(), c.Param("did"))
	if err != nil {
		return err
	}
	if d.MapID != mapID {
		return apperror.NewNotFound("drawing not found")
	}
	// A by-id read must not reveal what the list withholds; NotFound so the
	// answer matches a missing drawing.
	shadowed, err := h.drawingSvc.IsDrawingShadowed(c.Request().Context(), d, cc.VisibilityRole())
	if err != nil {
		return err
	}
	if shadowed {
		return apperror.NewNotFound("drawing not found")
	}
	return c.JSON(http.StatusOK, drawingResponseFor(c.Request().Context(), *d))
}

// UpdateDrawing updates an existing drawing.
// PUT /api/v1/campaigns/:id/maps/:mid/drawings/:did
func (h *DrawingHandler) UpdateDrawing(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	// PARTIAL update: absent preserves, explicit null clears, a present
	// value replaces (.ai/conventions.md).
	var req struct {
		Points            patch.Field[json.RawMessage] `json:"points"`
		StrokeColor       patch.Field[string]          `json:"stroke_color"`
		StrokeWidth       patch.Field[float64]         `json:"stroke_width"`
		FillColor         patch.Field[string]          `json:"fill_color"`
		FillAlpha         patch.Field[float64]         `json:"fill_alpha"`
		TextContent       patch.Field[string]          `json:"text_content"`
		FontSize          patch.Field[int]             `json:"font_size"`
		Rotation          patch.Field[float64]         `json:"rotation"`
		Visibility        patch.Field[string]          `json:"visibility"`
		ImageID           patch.Field[string]          `json:"image_id"`
		Crop              patch.Field[json.RawMessage] `json:"crop"`
		SortOrder         patch.Field[int]             `json:"sort_order"`
		ExpectedUpdatedAt *time.Time                   `json:"expected_updated_at"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.drawingSvc.UpdateDrawing(c.Request().Context(), c.Param("did"), c.Param("mid"), int(cc.MemberRole), cc.CanAuthorDmOnly(), UpdateDrawingInput{
		Points:            req.Points,
		StrokeColor:       req.StrokeColor,
		StrokeWidth:       req.StrokeWidth,
		FillColor:         req.FillColor,
		FillAlpha:         req.FillAlpha,
		TextContent:       req.TextContent,
		FontSize:          req.FontSize,
		Rotation:          req.Rotation,
		Visibility:        req.Visibility,
		ImageID:           req.ImageID,
		Crop:              req.Crop,
		SortOrder:         req.SortOrder,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt,
	}); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// DeleteDrawing removes a drawing.
// DELETE /api/v1/campaigns/:id/maps/:mid/drawings/:did
func (h *DrawingHandler) DeleteDrawing(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.drawingSvc.DeleteDrawing(c.Request().Context(), c.Param("did"), c.Param("mid"), ParseExpectedUpdatedAt(c), getUserID(c), int(cc.MemberRole), cc.CanAuthorDmOnly()); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// --- Token Endpoints ---

// ListTokens returns all tokens for a map, filtered by the user's role.
// GET /api/v1/campaigns/:id/maps/:mid/tokens
func (h *DrawingHandler) ListTokens(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	role := cc.VisibilityRole()
	tokens, err := h.drawingSvc.ListTokens(c.Request().Context(), mapID, role)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, tokens)
}

// CreateToken places a new token on a map.
// POST /api/v1/campaigns/:id/maps/:mid/tokens
func (h *DrawingHandler) CreateToken(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
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
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	t, err := h.drawingSvc.CreateToken(c.Request().Context(), CreateTokenInput{
		MapID:          mapID,
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
		CreatedBy:      getUserID(c),
		FoundryID:      req.FoundryID,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, t)
}

// GetToken returns a single token by ID.
// GET /api/v1/campaigns/:id/maps/:mid/tokens/:tid
func (h *DrawingHandler) GetToken(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	t, err := h.drawingSvc.GetToken(c.Request().Context(), c.Param("tid"))
	if err != nil {
		return err
	}
	if t.MapID != mapID {
		return apperror.NewNotFound("token not found")
	}
	return c.JSON(http.StatusOK, t)
}

// UpdateToken updates a token's properties.
// PUT /api/v1/campaigns/:id/maps/:mid/tokens/:tid
func (h *DrawingHandler) UpdateToken(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	// PARTIAL update: absent preserves, explicit null clears, a present
	// value replaces (.ai/conventions.md). A drag PUT carrying only {x, y}
	// must not zero IsHidden, IsLocked, HP bars or aura/light/vision fields.
	var req struct {
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
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.drawingSvc.UpdateToken(c.Request().Context(), c.Param("tid"), c.Param("mid"), UpdateTokenInput{
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
	}); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// UpdateTokenPosition updates only the position of a token (optimized for drag).
// PATCH /api/v1/campaigns/:id/maps/:mid/tokens/:tid/position
func (h *DrawingHandler) UpdateTokenPosition(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		X                 float64    `json:"x"`
		Y                 float64    `json:"y"`
		ExpectedUpdatedAt *time.Time `json:"expected_updated_at"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.drawingSvc.UpdateTokenPosition(c.Request().Context(), c.Param("tid"), c.Param("mid"), UpdateTokenPositionInput{
		X:                 req.X,
		Y:                 req.Y,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt,
	}); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// DeleteToken removes a token.
// DELETE /api/v1/campaigns/:id/maps/:mid/tokens/:tid
func (h *DrawingHandler) DeleteToken(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.drawingSvc.DeleteToken(c.Request().Context(), c.Param("tid"), c.Param("mid"), ParseExpectedUpdatedAt(c)); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// --- Layer Endpoints ---

// ListLayers returns all layers for a map.
// GET /api/v1/campaigns/:id/maps/:mid/layers
func (h *DrawingHandler) ListLayers(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	layers, err := h.drawingSvc.ListLayers(c.Request().Context(), mapID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, layers)
}

// CreateLayer creates a new layer on a map.
// POST /api/v1/campaigns/:id/maps/:mid/layers
func (h *DrawingHandler) CreateLayer(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		Name      string  `json:"name"`
		LayerType string  `json:"layer_type"`
		SortOrder int     `json:"sort_order"`
		IsVisible bool    `json:"is_visible"`
		Opacity   float64 `json:"opacity"`
		IsLocked  bool    `json:"is_locked"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	l, err := h.drawingSvc.CreateLayer(c.Request().Context(), CreateLayerInput{
		MapID:     mapID,
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
	return c.JSON(http.StatusCreated, l)
}

// GetLayer returns a single layer by ID.
// GET /api/v1/campaigns/:id/maps/:mid/layers/:lid
func (h *DrawingHandler) GetLayer(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	l, err := h.drawingSvc.GetLayer(c.Request().Context(), c.Param("lid"))
	if err != nil {
		return err
	}
	if l.MapID != mapID {
		return apperror.NewNotFound("layer not found")
	}
	return c.JSON(http.StatusOK, l)
}

// UpdateLayer updates a layer's properties.
// PUT /api/v1/campaigns/:id/maps/:mid/layers/:lid
func (h *DrawingHandler) UpdateLayer(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	// PARTIAL update: absent preserves, explicit null clears, a present
	// value replaces (.ai/conventions.md). A SortOrder-only PUT must not
	// turn other layers' visibility/lock off.
	var req struct {
		Name              string               `json:"name"`
		SortOrder         patch.Field[int]     `json:"sort_order"`
		IsVisible         patch.Field[bool]    `json:"is_visible"`
		Opacity           patch.Field[float64] `json:"opacity"`
		IsLocked          patch.Field[bool]    `json:"is_locked"`
		ExpectedUpdatedAt *time.Time           `json:"expected_updated_at"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.drawingSvc.UpdateLayer(c.Request().Context(), c.Param("lid"), c.Param("mid"), UpdateLayerInput{
		Name:              req.Name,
		SortOrder:         req.SortOrder,
		IsVisible:         req.IsVisible,
		Opacity:           req.Opacity,
		IsLocked:          req.IsLocked,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt,
	}); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// DeleteLayer removes a layer.
// DELETE /api/v1/campaigns/:id/maps/:mid/layers/:lid
func (h *DrawingHandler) DeleteLayer(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.drawingSvc.DeleteLayer(c.Request().Context(), c.Param("lid"), c.Param("mid"), ParseExpectedUpdatedAt(c)); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// --- Fog Endpoints ---

// ListFog returns all fog regions for a map.
// GET /api/v1/campaigns/:id/maps/:mid/fog
func (h *DrawingHandler) ListFog(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	fog, err := h.drawingSvc.ListFog(c.Request().Context(), mapID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, fog)
}

// CreateFog creates a new fog region on a map.
// POST /api/v1/campaigns/:id/maps/:mid/fog
func (h *DrawingHandler) CreateFog(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		Points     json.RawMessage `json:"points"`
		IsExplored bool            `json:"is_explored"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	f, err := h.drawingSvc.CreateFog(c.Request().Context(), CreateFogInput{
		MapID:      mapID,
		Points:     req.Points,
		IsExplored: req.IsExplored,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, f)
}

// DeleteFog removes a fog region.
// DELETE /api/v1/campaigns/:id/maps/:mid/fog/:fid
func (h *DrawingHandler) DeleteFog(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.drawingSvc.DeleteFog(c.Request().Context(), c.Param("fid"), mapID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// ResetFog removes all fog regions for a map.
// POST /api/v1/campaigns/:id/maps/:mid/fog/reset
func (h *DrawingHandler) ResetFog(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	if err := h.requireMapOwnership(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.drawingSvc.ResetFog(c.Request().Context(), mapID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}
