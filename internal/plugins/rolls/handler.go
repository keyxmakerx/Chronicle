package rolls

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves the Table tab's rolls as JSON. It binds and calls the
// service; who may see and do what is the service's.
type Handler struct {
	svc Service
}

// NewHandler creates the rolls handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// maxBodyBytes bounds a request body: a roll is a formula and a label.
const maxBodyBytes = 4 << 10

// RegisterRoutes mounts the Table tab's routes. Every member may read the log
// and roll; the service narrows what each one sees.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	player := campaigns.RequireRole(campaigns.RolePlayer)
	cg.GET("/rolls", h.List, player)
	cg.POST("/rolls", h.Roll, player)
	cg.GET("/rolls/settings", h.Settings, player)
	cg.PUT("/rolls/settings", h.SetSettings, player)
}

// ViewerOf is who is asking, from the campaign context. A member with DM
// access counts as a Director, as on every page.
func ViewerOf(cc *campaigns.CampaignContext, c echo.Context) Viewer {
	v := Viewer{UserID: auth.GetUserID(c), Director: cc.VisibilityRole() >= int(campaigns.RoleOwner)}
	if s := auth.GetSession(c); s != nil {
		v.Name = s.Name
	}
	return v
}

func (h *Handler) viewer(c echo.Context) (string, Viewer, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return "", Viewer{}, apperror.NewMissingContext()
	}
	return cc.Campaign.ID, ViewerOf(cc, c), nil
}

func bind(c echo.Context, into any) error {
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxBodyBytes)
	if err := c.Bind(into); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	return nil
}

// List returns a page of the log, oldest first: ?before=<roll id>&limit=N.
func (h *Handler) List(c echo.Context) error {
	campaignID, v, err := h.viewer(c)
	if err != nil {
		return err
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	page, err := h.svc.List(c.Request().Context(), campaignID, v, c.QueryParam("before"), limit)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, page)
}

// Roll makes a roll at the table and returns it as the roller sees it.
func (h *Handler) Roll(c echo.Context) error {
	campaignID, v, err := h.viewer(c)
	if err != nil {
		return err
	}
	var in TableInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Table(c.Request().Context(), campaignID, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// Settings returns the table's dice look.
func (h *Handler) Settings(c echo.Context) error {
	campaignID, _, err := h.viewer(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Settings(c.Request().Context(), campaignID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// SetSettings changes the table's dice look; the service refuses anyone but
// a Director.
func (h *Handler) SetSettings(c echo.Context) error {
	campaignID, v, err := h.viewer(c)
	if err != nil {
		return err
	}
	var in Settings
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.SetSettings(c.Request().Context(), campaignID, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
