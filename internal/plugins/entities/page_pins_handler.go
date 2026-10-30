package entities

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// PagePinHandler serves the Pinned Pages widget and the Director's pin beside
// a page's name. It works out who is looking and leaves the rest to the
// service.
type PagePinHandler struct {
	svc PagePinService
}

// NewPagePinHandler creates the pins handler.
func NewPagePinHandler(svc PagePinService) *PagePinHandler {
	return &PagePinHandler{svc: svc}
}

// RegisterPagePinRoutes mounts the pins routes. The widget is public-capable,
// like the home it sits on; pinning for everyone is the Director's, and taking
// off your own pin needs a member.
func RegisterPagePinRoutes(e *echo.Echo, h *PagePinHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/pinned-pages", h.Widget, campaigns.RequireViewAccess())

	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	director := campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool {
		return cc.CanAuthorDmOnly()
	}, "only the campaign owner or a granted co-Director pins pages for everyone")
	cg.GET("/entities/:eid/pin", h.PinButton, director)
	cg.POST("/entities/:eid/pin", h.SetPin, director)
	cg.DELETE("/pinned-pages/everyone/:eid", h.UnpinShared, director)
	cg.DELETE("/pinned-pages/mine/:eid", h.UnpinOwn, campaigns.RequireRole(campaigns.RolePlayer))
}

// pinViewer is who is looking. A Director looking as a player reads pages as a
// player and gets no Director controls, so the home shows what players see.
func pinViewer(c echo.Context, cc *campaigns.CampaignContext) PinViewer {
	v := PinViewer{
		CampaignID: cc.Campaign.ID,
		UserID:     auth.GetUserID(c),
		Role:       cc.VisibilityRole(),
		Director:   cc.CanAuthorDmOnly(),
	}
	if campaigns.ViewingAsPlayer(c, cc) {
		v.Role = int(campaigns.RolePlayer)
		v.Director = false
	}
	return v
}

// Widget draws the Pinned Pages widget for the viewer.
func (h *PagePinHandler) Widget(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	return h.renderWidget(c, cc, pinViewer(c, cc))
}

func (h *PagePinHandler) renderWidget(c echo.Context, cc *campaigns.CampaignContext, v PinViewer) error {
	pages, err := h.svc.ForViewer(c.Request().Context(), v)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, PinnedPagesWidget(cc, pages, v.Director))
}

// PinButton draws the Director's pin beside a page's name in its current
// state. Without the Director's pins table there is no pin to offer, and
// the empty answer leaves the page as it is.
func (h *PagePinHandler) PinButton(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if !h.svc.SharedAvailable() {
		return c.NoContent(http.StatusNoContent)
	}
	pinned, err := h.svc.IsShared(c.Request().Context(), cc.Campaign.ID, c.Param("eid"))
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, PagePinButton(cc, c.Param("eid"), pinned))
}

// SetPin pins the page for everyone (on=1) or takes it off, and answers with
// the pin in its new state.
func (h *PagePinHandler) SetPin(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	on := c.FormValue("on") == "1"
	if err := h.svc.SetShared(c.Request().Context(), pinViewer(c, cc), c.Param("eid"), on); err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, PagePinButton(cc, c.Param("eid"), on))
}

// UnpinShared takes a page off everyone's pins from the widget's ×.
func (h *PagePinHandler) UnpinShared(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v := pinViewer(c, cc)
	if err := h.svc.SetShared(c.Request().Context(), v, c.Param("eid"), false); err != nil {
		return err
	}
	return h.renderWidget(c, cc, v)
}

// UnpinOwn takes a page off the viewer's own pins from the widget's ×.
func (h *PagePinHandler) UnpinOwn(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v := pinViewer(c, cc)
	if err := h.svc.RemoveOwn(c.Request().Context(), v, c.Param("eid")); err != nil {
		return err
	}
	return h.renderWidget(c, cc, v)
}

// pinVals is the form value a pin click sends: the state it asks for.
func pinVals(on bool) string {
	if on {
		return `{"on":"1"}`
	}
	return `{"on":"0"}`
}
