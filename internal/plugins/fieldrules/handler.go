package fieldrules

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves the homebrew tools' JSON. It binds and calls the service;
// who may read or change what is the service's.
type Handler struct{ svc Service }

// NewHandler creates the handler.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

// maxBodyBytes bounds a request: a rule of MaxRuleRows short rows.
const maxBodyBytes = 32 << 10

// directorOnly is the routes' early refusal; the service checks again.
func directorOnly() echo.MiddlewareFunc {
	return campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool {
		return cc.CanAuthorDmOnly()
	}, "only the Director can see and change homebrew tools")
}

// RegisterRoutes mounts the API under /campaigns/:id/homebrew.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id/homebrew",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		directorOnly(),
	)
	cg.GET("/rules", h.ListRules)
	cg.POST("/rules", h.SaveRule)
	cg.PUT("/rules/:rid", h.SaveRule)
	cg.DELETE("/rules/:rid", h.DeleteRule)
	cg.GET("/words", h.ListWords)
	cg.POST("/words", h.SaveWord)
	cg.PUT("/words/:wid", h.SaveWord)
	cg.DELETE("/words/:wid", h.DeleteWord)
}

// ViewerOf is who is asking, by visibility role so a granted co-Director
// counts as the Director.
func ViewerOf(cc *campaigns.CampaignContext, userID string) Viewer {
	return Viewer{UserID: userID, Role: cc.VisibilityRole()}
}

func (h *Handler) ctx(c echo.Context) (*campaigns.CampaignContext, Viewer, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return nil, Viewer{}, apperror.NewMissingContext()
	}
	return cc, ViewerOf(cc, auth.GetUserID(c)), nil
}

func bind(c echo.Context, into any) error {
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxBodyBytes)
	if err := c.Bind(into); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	return nil
}

// idParam reads a numeric path id; 0 when the route has none (a create).
func idParam(c echo.Context, name string) (int, error) {
	raw := c.Param(name)
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, apperror.NewNotFound("not found")
	}
	return id, nil
}

// ListRules returns every rule.
func (h *Handler) ListRules(c echo.Context) error {
	cc, v, err := h.ctx(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Rules(c.Request().Context(), cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"rules": out})
}

type ruleBody struct {
	Name    string    `json:"name"`
	Follows string    `json:"follows"`
	Rows    []RuleRow `json:"rows"`
}

// SaveRule creates (POST) or replaces (PUT) a rule.
func (h *Handler) SaveRule(c echo.Context) error {
	cc, v, err := h.ctx(c)
	if err != nil {
		return err
	}
	id, err := idParam(c, "rid")
	if err != nil {
		return err
	}
	var b ruleBody
	if err := bind(c, &b); err != nil {
		return err
	}
	out, err := h.svc.SaveRule(c.Request().Context(), cc.Campaign.ID, RuleInput{ID: id, Name: b.Name, Follows: b.Follows, Rows: b.Rows}, v)
	if err != nil {
		return err
	}
	status := http.StatusOK
	if id == 0 {
		status = http.StatusCreated
	}
	return c.JSON(status, out)
}

// DeleteRule removes a rule. Fields that followed it keep any value typed by
// hand and otherwise show nothing.
func (h *Handler) DeleteRule(c echo.Context) error {
	cc, v, err := h.ctx(c)
	if err != nil {
		return err
	}
	id, err := idParam(c, "rid")
	if err != nil {
		return err
	}
	if err := h.svc.DeleteRule(c.Request().Context(), cc.Campaign.ID, id, v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ListWords returns every local word.
func (h *Handler) ListWords(c echo.Context) error {
	cc, v, err := h.ctx(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Words(c.Request().Context(), cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"words": out})
}

type wordBody struct {
	Word    string `json:"word"`
	Local   string `json:"local"`
	PlaceID string `json:"placeId"`
	EraID   int    `json:"eraId"`
	DMOnly  bool   `json:"dmOnly"`
}

// SaveWord creates (POST) or replaces (PUT) a local word.
func (h *Handler) SaveWord(c echo.Context) error {
	cc, v, err := h.ctx(c)
	if err != nil {
		return err
	}
	id, err := idParam(c, "wid")
	if err != nil {
		return err
	}
	var b wordBody
	if err := bind(c, &b); err != nil {
		return err
	}
	out, err := h.svc.SaveWord(c.Request().Context(), cc.Campaign.ID,
		WordInput{ID: id, Word: b.Word, Local: b.Local, PlaceID: b.PlaceID, EraID: b.EraID, DMOnly: b.DMOnly}, v)
	if err != nil {
		return err
	}
	status := http.StatusOK
	if id == 0 {
		status = http.StatusCreated
	}
	return c.JSON(status, out)
}

// DeleteWord removes a local word.
func (h *Handler) DeleteWord(c echo.Context) error {
	cc, v, err := h.ctx(c)
	if err != nil {
		return err
	}
	id, err := idParam(c, "wid")
	if err != nil {
		return err
	}
	if err := h.svc.DeleteWord(c.Request().Context(), cc.Campaign.ID, id, v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
