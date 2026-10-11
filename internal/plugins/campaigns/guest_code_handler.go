package campaigns

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// GuestCodeHandler serves the owner's Guest codes card.
type GuestCodeHandler struct {
	svc     *GuestCodeService
	baseURL string
}

// NewGuestCodeHandler returns the guest code handler.
func NewGuestCodeHandler(svc *GuestCodeService, baseURL string) *GuestCodeHandler {
	return &GuestCodeHandler{svc: svc, baseURL: strings.TrimRight(baseURL, "/")}
}

// RegisterGuestCodeRoutes adds the owner's guest code routes.
func RegisterGuestCodeRoutes(e *echo.Echo, h *GuestCodeHandler, svc CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id", auth.RequireAuth(authSvc), RequireCampaignAccess(svc))
	cg.GET("/guest-codes", h.Card, RequireRole(RoleOwner))
	cg.POST("/guest-codes", h.Create, RequireRole(RoleOwner), middleware.RateLimit(30, time.Minute))
	cg.DELETE("/guest-codes/:gcid", h.Remove, RequireRole(RoleOwner))
}

// Card renders the card (GET /campaigns/:id/guest-codes).
func (h *GuestCodeHandler) Card(c echo.Context) error {
	return h.render(c, nil, "")
}

// Create makes a code and shows it once (POST /campaigns/:id/guest-codes).
func (h *GuestCodeHandler) Create(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	fresh, err := h.svc.Create(c.Request().Context(), cc.Campaign.ID, auth.GetUserID(c), c.FormValue("note"))
	if err != nil {
		return h.render(c, nil, apperror.UserMessage(err, "the code couldn't be made"))
	}
	return h.render(c, fresh, "")
}

// Remove deletes an unused code (DELETE /campaigns/:id/guest-codes/:gcid).
func (h *GuestCodeHandler) Remove(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if err := h.svc.Remove(c.Request().Context(), cc.Campaign.ID, c.Param("gcid")); err != nil {
		return h.render(c, nil, apperror.UserMessage(err, "the code couldn't be removed"))
	}
	return h.render(c, nil, "")
}

func (h *GuestCodeHandler) render(c echo.Context, fresh *NewGuestCode, errMsg string) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	codes, err := h.svc.List(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, GuestCodesCard(cc.Campaign.ID, codes, fresh, h.svc.Now(), h.baseURL+"/join", middleware.GetCSRFToken(c), errMsg))
}
