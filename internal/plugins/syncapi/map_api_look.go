// Package syncapi — map_api_look.go serves the campaign-wide map look to
// external clients, so the Foundry module can draw a map the way Chronicle
// does without a Chronicle login.
package syncapi

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
)

// mapLookResponse is what a client needs, besides each map's own
// display_settings, to resolve how a map looks: the campaign frame a map
// follows when it has no frame of its own, the closed sets it may name, and
// the canonical marker icons.
type mapLookResponse struct {
	CampaignFrame string             `json:"campaign_frame"`
	Frames        []string           `json:"frames"`
	Kinds         []maps.KindDisplay `json:"kinds"`
	Icons         []maps.MarkerIcon  `json:"icons"`
	DefaultIcon   string             `json:"default_icon"`
}

// GetMapLook returns the campaign's map look.
// GET /api/v1/campaigns/:id/maps/look
func (h *MapAPIHandler) GetMapLook(c echo.Context) error {
	frame, err := h.mapSvc.GetCampaignFrame(c.Request().Context(), c.Param("id"))
	if err != nil || !maps.IsValidFrame(frame) {
		// The look is cosmetic: a failed lookup draws the default frame
		// rather than failing the client's whole map sync.
		if err != nil {
			slog.Warn("api: campaign map frame lookup failed", slog.Any("error", err))
		}
		frame = maps.DefaultFrame
	}

	frames := make([]string, 0, len(maps.FrameStyles))
	for _, f := range maps.FrameStyles {
		frames = append(frames, f.ID)
	}
	kinds := make([]maps.KindDisplay, len(maps.KindDefaults))
	copy(kinds, maps.KindDefaults)

	return c.JSON(http.StatusOK, mapLookResponse{
		CampaignFrame: frame,
		Frames:        frames,
		Kinds:         kinds,
		Icons:         maps.MarkerIconCatalog(),
		DefaultIcon:   maps.DefaultMarkerIcon,
	})
}
