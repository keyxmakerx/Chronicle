package dmscreen

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves the DM Screen fragment and its reveal action.
type Handler struct {
	svc Service
}

// NewHandler returns a Handler backed by svc.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

func viewerOf(c echo.Context, cc *campaigns.CampaignContext) Viewer {
	return Viewer{UserID: auth.GetUserID(c), Role: cc.VisibilityRole()}
}

// Show handles GET /campaigns/:id/dm-screen and returns the panel fragment
// the top-bar widget mounts.
func (h *Handler) Show(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	view, err := h.svc.Build(c.Request().Context(), cc.Campaign.ID, viewerOf(c, cc))
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, Panel(view, middleware.GetCSRFToken(c)))
}

// Reveal handles POST /campaigns/:id/dm-screen/reveal/:eid and returns the
// row redrawn as revealed.
func (h *Handler) Reveal(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	entityID := c.Param("eid")
	name, err := h.svc.Reveal(c.Request().Context(), entityID, cc.Campaign.ID, viewerOf(c, cc))
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, HiddenRow(cc.Campaign.ID, HiddenView{ID: entityID, Name: name, Revealed: true}, ""))
}
