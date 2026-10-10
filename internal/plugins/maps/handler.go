package maps

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The map detail page lazy-loads /foundry-vtt/presence-pill-fragment from
// foundry_vtt, which owns the presence-pill UI; this plugin has no WS-hub
// presence access.

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
	// The cards show each map's picture, so they get the viewer's version of it.
	mapList, err = h.svc.ForViewerList(ctx, mapList, cc.VisibilityRole())
	if err != nil {
		return err
	}

	// A failed frame lookup must not take the list down: the cards fall back
	// to the default frame.
	frame, err := h.svc.GetCampaignFrame(ctx, cc.Campaign.ID)
	if err != nil {
		frame = DefaultFrame
	}

	data := MapListData{
		CampaignID:    cc.Campaign.ID,
		Maps:          mapList,
		IsOwner:       cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:      cc.MemberRole >= campaigns.RoleScribe,
		CSRFToken:     middleware.GetCSRFToken(c),
		CampaignFrame: frame,
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
	data, err := h.mapViewData(c, cc)
	if err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, MapShowFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, MapShowPage(cc, data))
}

// Viewer renders the bare framed viewer (no page chrome) that the focus view
// fetches when a map preview on another page is opened.
// GET /campaigns/:id/maps/:mid/viewer
//
// It is registered in the same group, with the same access check, as Show and
// builds its data through the same mapViewData, so what a viewer receives here
// (role-filtered markers, tool gates, resolved frame) is by construction what
// the map page gives them; there is no second copy of the rules to drift.
func (h *Handler) Viewer(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	data, err := h.mapViewData(c, cc)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, MapViewerFragment(cc, data))
}

// mapViewData loads everything the viewer needs for the :mid map, scoped to the
// request's campaign and filtered for the requester's role. Shared by the map
// page and the focus-view fragment so the two cannot disagree about who sees
// what.
func (h *Handler) mapViewData(c echo.Context, cc *campaigns.CampaignContext) (MapViewData, error) {
	mapID := c.Param("mid")

	m, err := h.requireMapInCampaign(c, mapID, cc.Campaign.ID)
	if err != nil {
		return MapViewData{}, err
	}

	role := cc.VisibilityRole()
	userID := getUserID(c)
	markers, err := h.svc.ListMarkers(c.Request().Context(), cc.Campaign.ID, mapID, role, userID)
	if err != nil {
		return MapViewData{}, err
	}

	// ResolveDisplay returns usable defaults alongside any error, so a failed
	// campaign-frame lookup degrades to the default frame instead of a 500.
	display, _ := h.svc.ResolveDisplay(c.Request().Context(), m)

	// From here on the page only ever holds the viewer's version of the map, so
	// no template can reach the original picture of a shadowed map.
	m, err = h.svc.ForViewer(c.Request().Context(), m, role)
	if err != nil {
		return MapViewData{}, err
	}

	return MapViewData{
		CampaignID: cc.Campaign.ID,
		Map:        m,
		Markers:    markers,
		IsScribe:   cc.MemberRole >= campaigns.RoleScribe,
		IsOwner:    cc.MemberRole >= campaigns.RoleOwner,
		IsDM:       cc.CanAuthorDmOnly(),
		UserID:     userID,
		Display:    display,
	}, nil
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

	// PARTIAL update: absent preserves, explicit null clears, a present
	// value replaces (.ai/conventions.md). A rename-only PUT must not
	// unlink the map's image or wipe its description.
	var req struct {
		Name        string              `json:"name"`
		Description patch.Field[string] `json:"description"`
		ImageID     patch.Field[string] `json:"image_id"`
		ImageWidth  patch.Field[int]    `json:"image_width"`
		ImageHeight patch.Field[int]    `json:"image_height"`
		// background_color is tri-state: omitted (nil) leaves the
		// stored value unchanged; "" clears the override (revert to
		// theme); any CSS color sets the override.
		BackgroundColor *string `json:"background_color"`
		// display_settings follows the same contract, by group: absent keeps
		// the stored settings, null clears them all, and an object replaces
		// only the groups it names (see MergeDisplaySettings).
		DisplaySettings   patch.Field[json.RawMessage] `json:"display_settings"`
		ExpectedUpdatedAt *time.Time                   `json:"expected_updated_at"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	return h.svc.UpdateMap(ctx, mapID, UpdateMapInput{
		Name:              req.Name,
		Description:       req.Description,
		ImageID:           req.ImageID,
		ImageWidth:        req.ImageWidth,
		ImageHeight:       req.ImageHeight,
		BackgroundColor:   req.BackgroundColor,
		DisplaySettings:   req.DisplaySettings,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt,
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

	if err := h.svc.DeleteMap(ctx, mapID, ParseExpectedUpdatedAt(c)); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// ParseExpectedUpdatedAt extracts the optional optimistic-concurrency
// token. Callers may send it via a JSON body field on DELETE requests
// (`{"expected_updated_at": "..."}`) or via the `expected_updated_at`
// query parameter — both are accepted because curl-style DELETE often
// avoids bodies. Invalid values fall through to nil (last-writer-wins)
// rather than 400 — the field is advisory.
func ParseExpectedUpdatedAt(c echo.Context) *time.Time {
	if q := c.QueryParam("expected_updated_at"); q != "" {
		if t, err := time.Parse(time.RFC3339Nano, q); err == nil {
			return &t
		}
	}
	var body struct {
		ExpectedUpdatedAt *time.Time `json:"expected_updated_at"`
	}
	if err := c.Bind(&body); err == nil && body.ExpectedUpdatedAt != nil {
		return body.ExpectedUpdatedAt
	}
	return nil
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
	userID := getUserID(c)

	// Only a caller who can author dm_only content (Owner or a co-DM grant)
	// may create a dm_only marker; anyone else defaults to 'everyone'.
	visibility := req.Visibility
	if visibility == "dm_only" && !cc.CanAuthorDmOnly() && !cc.IsSiteAdmin {
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

	// PARTIAL update: absent preserves, explicit null clears, a present value
	// replaces (.ai/conventions.md). The edit form and the drag-end PUT each
	// send only a subset of fields.
	//
	// foundry_id is deliberately NOT a member here: a browser form has no
	// business setting or clearing a sync pairing key. Absent-preserve stops
	// the web edit from nulling it; syncapi still clears one via an explicit
	// null.
	var req struct {
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
		ExpectedUpdatedAt *time.Time           `json:"expected_updated_at"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	// Only a caller who can author dm_only content (Owner or a co-DM grant)
	// may set dm_only visibility; anyone else defaults to 'everyone'. The
	// downgrade applies only to a visibility the caller actually SENT.
	canAuthorDmOnly := cc.CanAuthorDmOnly() || cc.IsSiteAdmin
	visibility := req.Visibility
	if v, ok := req.Visibility.Get(); ok && v == "dm_only" && !canAuthorDmOnly {
		visibility = patch.Of("everyone")
	}
	// Only Owners can set per-player visibility rules. A non-Owner's request
	// is dropped to ABSENT, not to null: refusing a write is not authority to
	// erase the Owner's existing rules, which is what it used to do.
	visRules := req.VisibilityRules
	if cc.MemberRole < campaigns.RoleOwner && !cc.IsSiteAdmin {
		visRules = patch.Absent[string]()
	}

	return h.svc.UpdateMarker(ctx, markerID, UpdateMarkerInput{
		Name:              req.Name,
		Description:       req.Description,
		X:                 req.X,
		Y:                 req.Y,
		Icon:              req.Icon,
		Color:             req.Color,
		PinCategory:       req.PinCategory,
		EntityID:          req.EntityID,
		Visibility:        visibility,
		VisibilityRules:   visRules,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt,
	}, canAuthorDmOnly)
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

	canAuthorDmOnly := cc.CanAuthorDmOnly() || cc.IsSiteAdmin
	if err := h.svc.DeleteMarker(ctx, markerID, ParseExpectedUpdatedAt(c), canAuthorDmOnly, getUserID(c), int(cc.MemberRole)); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// GetMapMetaAPI returns the public-capable JSON the embeddable map widget needs
// (image id + dimensions + visibility-filtered markers) so map-widget /
// entity-map blocks render on public campaigns without the /api/v1 (Foundry /
// API-key) path. Role/visibility filtering matches ListMarkersAPI and is
// empty-userID safe (anonymous public visitors).
// GET /campaigns/:id/maps/:mid/meta
func (h *Handler) GetMapMetaAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	mapID := c.Param("mid")

	m, err := h.requireMapInCampaign(c, mapID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	role := cc.VisibilityRole()
	userID := getUserID(c)
	markers, err := h.svc.ListMarkers(ctx, cc.Campaign.ID, mapID, role, userID)
	if err != nil {
		return err
	}
	if markers == nil {
		markers = []Marker{}
	}
	m, err = h.svc.ForViewer(ctx, m, role)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"id":           m.ID,
		"name":         m.Name,
		"updated_at":   m.UpdatedAt,
		"image_id":     m.ImageID,
		"image_url":    m.PlayerImageURL,
		"image_width":  m.ImageWidth,
		"image_height": m.ImageHeight,
		"markers":      markers,
	})
}

// PlayerImage serves the map's picture with its shadowed areas smudged into the
// pixels, for viewers who must not receive the original. It is registered beside
// the map page with the same access check, and any failure to produce the copy
// is an error, never the original.
// GET /campaigns/:id/maps/:mid/player-image
func (h *Handler) PlayerImage(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	m, err := h.requireMapInCampaign(c, c.Param("mid"), cc.Campaign.ID)
	if err != nil {
		return err
	}
	data, err := h.svc.PlayerImage(c.Request().Context(), m)
	if err != nil {
		return err
	}
	// The address carries the shadow version, so a copy never goes stale; private
	// because it is for this campaign's viewers only.
	c.Response().Header().Set("Cache-Control", "private, max-age=86400")
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	return c.Blob(http.StatusOK, "image/jpeg", data)
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
	userID := getUserID(c)

	markers, err := h.svc.ListMarkers(ctx, cc.Campaign.ID, mapID, role, userID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, markers)
}

// MarkerIconsAPI returns Chronicle's canonical map-marker icon vocabulary.
// Chronicle is authoritative for the icon set; this endpoint is the contract
// the Foundry sync module reads to align its translation table, so the same
// icon ID renders the same concept on both sides. Static catalog (no
// per-campaign state) but campaign-scoped + Player-gated to match the rest
// of the maps API surface.
func (h *Handler) MarkerIconsAPI(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"default": DefaultMarkerIcon,
		"icons":   MarkerIconCatalog(),
		"groups":  MarkerIconGroups(),
	})
}

// SetCampaignFrameAPI stores the campaign-wide map frame, chosen on the
// Customize page's Maps tab (Owner only, enforced by the route). It answers an
// HTMX request with the refreshed section so "Saved" shows in place, and a
// plain request with 204 so the endpoint is also usable without the page.
// PUT /campaigns/:id/maps/frame-style
func (h *Handler) SetCampaignFrameAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	// The page posts a form; an API caller may send JSON. Bind handles both.
	var req struct {
		Frame string `json:"frame" form:"frame"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if err := h.svc.SetCampaignFrame(ctx, cc.Campaign.ID, req.Frame); err != nil {
		return err
	}
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, mapFrameSection(cc.Campaign.ID, req.Frame, true))
	}
	return c.NoContent(http.StatusNoContent)
}
