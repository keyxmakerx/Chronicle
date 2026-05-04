package maps

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler processes HTTP requests for the maps plugin.
type Handler struct {
	svc MapService
}

// NewHandler creates a new maps Handler.
func NewHandler(svc MapService) *Handler {
	return &Handler{svc: svc}
}

// Index lists all maps for a campaign, or redirects to the first map.
// GET /campaigns/:id/maps
func (h *Handler) Index(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	mapList, err := h.svc.ListMaps(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}

	data := MapListData{
		CampaignID: cc.Campaign.ID,
		Maps:       mapList,
		IsOwner:    cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:   cc.MemberRole >= campaigns.RoleScribe,
		CSRFToken:  middleware.GetCSRFToken(c),
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, MapListFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, MapListPage(cc, data))
}

// requireMapInCampaign fetches a map by ID and verifies it belongs to the
// given campaign. Returns 404 if not found or mismatched, preventing
// cross-campaign IDOR attacks.
func (h *Handler) requireMapInCampaign(c echo.Context, mapID, campaignID string) (*Map, error) {
	return middleware.RequireInCampaign(c.Request().Context(), h.svc.GetMap, mapID, campaignID, "map")
}

// requireMarkerInCampaign fetches a marker and verifies its parent map belongs
// to the given campaign. Returns 404 for cross-campaign IDOR attempts.
func (h *Handler) requireMarkerInCampaign(c echo.Context, markerID, campaignID string) (*Marker, error) {
	mk, err := h.svc.GetMarker(c.Request().Context(), markerID)
	if err != nil {
		return nil, err
	}
	// Verify the marker's parent map belongs to the correct campaign.
	m, err := h.svc.GetMap(c.Request().Context(), mk.MapID)
	if err != nil || m.CampaignID != campaignID {
		return nil, apperror.NewNotFound("marker not found")
	}
	return mk, nil
}

// Show renders a single map with its markers in a Leaflet.js viewer.
// GET /campaigns/:id/maps/:mid
func (h *Handler) Show(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	mapID := c.Param("mid")

	m, err := h.requireMapInCampaign(c, mapID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	role := cc.VisibilityRole()
	userID := ""
	if session := c.Get("session"); session != nil {
		if s, ok := session.(interface{ GetUserID() string }); ok {
			userID = s.GetUserID()
		}
	}
	markers, err := h.svc.ListMarkers(c.Request().Context(), mapID, role, userID)
	if err != nil {
		return err
	}

	data := MapViewData{
		CampaignID: cc.Campaign.ID,
		Map:        m,
		Markers:    markers,
		IsScribe:   cc.MemberRole >= campaigns.RoleScribe,
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, MapShowFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, MapShowPage(cc, data))
}

// CreateMapAPI creates a new map.
// POST /campaigns/:id/maps
func (h *Handler) CreateMapAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		ImageID     *string `json:"image_id"`
		ImageWidth  int     `json:"image_width"`
		ImageHeight int     `json:"image_height"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	// Validate field lengths.
	if err := apperror.ValidateRequired("name", req.Name); err != nil {
		return err
	}
	if err := apperror.ValidateStringLength("name", req.Name, apperror.MaxNameLength); err != nil {
		return err
	}

	m, err := h.svc.CreateMap(ctx, CreateMapInput{
		CampaignID:  cc.Campaign.ID,
		Name:        req.Name,
		Description: req.Description,
		ImageID:     req.ImageID,
		ImageWidth:  req.ImageWidth,
		ImageHeight: req.ImageHeight,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, m)
}

// CreateMapForm handles map creation from a form POST.
// POST /campaigns/:id/maps/new
func (h *Handler) CreateMapForm(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	name := c.FormValue("name")
	if name == "" {
		name = "New Map"
	}
	desc := c.FormValue("description")
	var descPtr *string
	if desc != "" {
		descPtr = &desc
	}

	m, err := h.svc.CreateMap(ctx, CreateMapInput{
		CampaignID:  cc.Campaign.ID,
		Name:        name,
		Description: descPtr,
	})
	if err != nil {
		return err
	}

	return c.Redirect(http.StatusSeeOther,
		fmt.Sprintf("/campaigns/%s/maps/%s", cc.Campaign.ID, m.ID))
}

// UpdateMapAPI updates a map's metadata.
// PUT /campaigns/:id/maps/:mid
func (h *Handler) UpdateMapAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	mapID := c.Param("mid")

	// IDOR protection: verify map belongs to this campaign.
	if _, err := h.requireMapInCampaign(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
		Name            string  `json:"name"`
		Description     *string `json:"description"`
		ImageID         *string `json:"image_id"`
		ImageWidth      int     `json:"image_width"`
		ImageHeight     int     `json:"image_height"`
		// background_color is tri-state: omitted (nil) leaves the
		// stored value unchanged; "" clears the override (revert to
		// theme); any CSS color sets the override.
		BackgroundColor *string `json:"background_color"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	return h.svc.UpdateMap(ctx, mapID, UpdateMapInput{
		Name:            req.Name,
		Description:     req.Description,
		ImageID:         req.ImageID,
		ImageWidth:      req.ImageWidth,
		ImageHeight:     req.ImageHeight,
		BackgroundColor: req.BackgroundColor,
	})
}

// DeleteMapAPI deletes a map and all its markers.
// DELETE /campaigns/:id/maps/:mid
func (h *Handler) DeleteMapAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	mapID := c.Param("mid")

	// IDOR protection: verify map belongs to this campaign.
	if _, err := h.requireMapInCampaign(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.svc.DeleteMap(ctx, mapID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// CreateMarkerAPI places a new marker on a map.
// POST /campaigns/:id/maps/:mid/markers
func (h *Handler) CreateMarkerAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	mapID := c.Param("mid")

	// IDOR protection: verify map belongs to this campaign.
	if _, err := h.requireMapInCampaign(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
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
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	// Get user ID from session context.
	userID := ""
	if session := c.Get("session"); session != nil {
		if s, ok := session.(interface{ GetUserID() string }); ok {
			userID = s.GetUserID()
		}
	}

	// Only Owners can create dm_only markers; Scribes default to 'everyone'.
	visibility := req.Visibility
	if visibility == "dm_only" && cc.MemberRole < campaigns.RoleOwner && !cc.IsSiteAdmin {
		visibility = "everyone"
	}
	// Only Owners can set per-player visibility rules.
	var visRules *string
	if cc.MemberRole >= campaigns.RoleOwner || cc.IsSiteAdmin {
		visRules = req.VisibilityRules
	}

	mk, err := h.svc.CreateMarker(ctx, CreateMarkerInput{
		MapID:           mapID,
		Name:            req.Name,
		Description:     req.Description,
		X:               req.X,
		Y:               req.Y,
		Icon:            req.Icon,
		Color:           req.Color,
		PinCategory:     req.PinCategory,
		EntityID:        req.EntityID,
		Visibility:      visibility,
		VisibilityRules: visRules,
		CreatedBy:       userID,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, mk)
}

// UpdateMarkerAPI updates an existing marker.
// PUT /campaigns/:id/maps/:mid/markers/:mkid
func (h *Handler) UpdateMarkerAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	markerID := c.Param("mkid")

	// IDOR protection: verify marker's parent map belongs to this campaign.
	if _, err := h.requireMarkerInCampaign(c, markerID, cc.Campaign.ID); err != nil {
		return err
	}

	var req struct {
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
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	// Only Owners can set dm_only visibility; Scribes default to 'everyone'.
	visibility := req.Visibility
	if visibility == "dm_only" && cc.MemberRole < campaigns.RoleOwner && !cc.IsSiteAdmin {
		visibility = "everyone"
	}
	// Only Owners can set per-player visibility rules.
	var visRules *string
	if cc.MemberRole >= campaigns.RoleOwner || cc.IsSiteAdmin {
		visRules = req.VisibilityRules
	}

	return h.svc.UpdateMarker(ctx, markerID, UpdateMarkerInput{
		Name:            req.Name,
		Description:     req.Description,
		X:               req.X,
		Y:               req.Y,
		Icon:            req.Icon,
		Color:           req.Color,
		PinCategory:     req.PinCategory,
		EntityID:        req.EntityID,
		Visibility:      visibility,
		VisibilityRules: visRules,
	})
}

// DeleteMarkerAPI deletes a marker.
// DELETE /campaigns/:id/maps/:mid/markers/:mkid
func (h *Handler) DeleteMarkerAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	markerID := c.Param("mkid")

	// IDOR protection: verify marker's parent map belongs to this campaign.
	if _, err := h.requireMarkerInCampaign(c, markerID, cc.Campaign.ID); err != nil {
		return err
	}

	if err := h.svc.DeleteMarker(ctx, markerID); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// ListMarkersAPI returns all markers for a map as JSON.
// GET /campaigns/:id/maps/:mid/markers
func (h *Handler) ListMarkersAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	mapID := c.Param("mid")

	// IDOR protection: verify map belongs to this campaign.
	if _, err := h.requireMapInCampaign(c, mapID, cc.Campaign.ID); err != nil {
		return err
	}

	role := cc.VisibilityRole()
	userID := ""
	if session := c.Get("session"); session != nil {
		if s, ok := session.(interface{ GetUserID() string }); ok {
			userID = s.GetUserID()
		}
	}

	markers, err := h.svc.ListMarkers(ctx, mapID, role, userID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, markers)
}
