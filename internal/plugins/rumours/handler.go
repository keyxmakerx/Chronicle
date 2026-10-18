package rumours

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Pages answers whether the caller may open a page. It is owned here and
// implemented in internal/app over the entities service, so this plugin never
// imports another plugin. A page that is missing, in another campaign or
// hidden from the caller is the same answer: not found.
type Pages interface {
	Open(ctx context.Context, campaignID, entityID string, role int, userID string) error
}

// Handler serves the Rumours block's JSON. It binds, checks the page is one
// the caller may open, and calls the service; every rule about who may see or
// write what is the service's.
type Handler struct {
	svc   Service
	pages Pages
}

// NewHandler creates the Rumours handler.
func NewHandler(svc Service, pages Pages) *Handler {
	return &Handler{svc: svc, pages: pages}
}

// maxBodyBytes bounds a request body: a rumour is a few hundred characters.
const maxBodyBytes = 8 << 10

// scribeOrDirector is the routes' early refusal for writes. The service checks
// the role again, so this is the cheap refusal and the service is the one that
// cannot be skipped. A granted co-Director counts as the Director.
func scribeOrDirector() echo.MiddlewareFunc {
	return campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool {
		return cc.CanAuthorDmOnly() || cc.MemberRole >= campaigns.RoleScribe
	}, "only the Director and Scribes can change rumours")
}

// directorOnly gates the one write a Scribe must never make.
func directorOnly() echo.MiddlewareFunc {
	return campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool {
		return cc.CanAuthorDmOnly()
	}, "only the Director can say what is true")
}

// RegisterRoutes mounts the block's routes. Reads need a campaign member and
// the service narrows them further; writes are the Director's and Scribes'.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	player := campaigns.RequireRole(campaigns.RolePlayer)
	cg.GET("/entities/:eid/rumours", h.List, player)
	cg.POST("/entities/:eid/rumours", h.Add, scribeOrDirector())
	cg.PUT("/entities/:eid/rumours/:rid", h.Update, scribeOrDirector())
	cg.PUT("/entities/:eid/rumours/:rid/truth", h.SetTruth, directorOnly())
	cg.PUT("/entities/:eid/rumours/:rid/belief", h.SetBelief, scribeOrDirector())
	cg.DELETE("/entities/:eid/rumours/:rid", h.Remove, scribeOrDirector())
}

// ViewerOf is who is asking, from the campaign context. VisibilityRole is
// used as on the page itself, so a granted co-Director sees and may do what
// the owner does.
func ViewerOf(cc *campaigns.CampaignContext, userID string) Viewer {
	return Viewer{UserID: userID, Role: cc.VisibilityRole()}
}

// page loads the campaign context and checks the caller may open the page.
func (h *Handler) page(c echo.Context) (*campaigns.CampaignContext, Viewer, string, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return nil, Viewer{}, "", apperror.NewMissingContext()
	}
	v := ViewerOf(cc, auth.GetUserID(c))
	eid := c.Param("eid")
	if err := h.pages.Open(c.Request().Context(), cc.Campaign.ID, eid, v.Role, v.UserID); err != nil {
		return nil, Viewer{}, "", err
	}
	return cc, v, eid, nil
}

func bind(c echo.Context, into any) error {
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxBodyBytes)
	if err := c.Bind(into); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	return nil
}

// List returns the page's rumours as the caller may see them.
func (h *Handler) List(c echo.Context) error {
	cc, v, eid, err := h.page(c)
	if err != nil {
		return err
	}
	out, err := h.svc.View(c.Request().Context(), cc.Campaign.ID, eid, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// addBody is the JSON a new rumour arrives as.
type addBody struct {
	Quote string `json:"quote"`
	Who   string `json:"who"`
	Sure  int    `json:"sure"`
}

// Add writes a rumour and returns it.
func (h *Handler) Add(c echo.Context) error {
	cc, v, eid, err := h.page(c)
	if err != nil {
		return err
	}
	var b addBody
	if err := bind(c, &b); err != nil {
		return err
	}
	out, err := h.svc.Add(c.Request().Context(), AddInput{
		CampaignID: cc.Campaign.ID, EntityID: eid, Quote: b.Quote, Who: b.Who, Sure: b.Sure, Viewer: v,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// updateBody is the JSON an edit arrives as; a key left out keeps its value.
type updateBody struct {
	Quote patch.Field[string] `json:"quote"`
	Who   patch.Field[string] `json:"who"`
	Sure  patch.Field[int]    `json:"sure"`
}

// Update edits a rumour's words and returns it.
func (h *Handler) Update(c echo.Context) error {
	cc, v, eid, err := h.page(c)
	if err != nil {
		return err
	}
	var b updateBody
	if err := bind(c, &b); err != nil {
		return err
	}
	out, err := h.svc.Update(c.Request().Context(), cc.Campaign.ID, eid, c.Param("rid"),
		UpdateRumourInput(b), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// truthBody is the JSON the Director's verdict arrives as.
type truthBody struct {
	Truth string `json:"truth"`
}

// SetTruth records what is true and returns the rumour.
func (h *Handler) SetTruth(c echo.Context) error {
	cc, v, eid, err := h.page(c)
	if err != nil {
		return err
	}
	var b truthBody
	if err := bind(c, &b); err != nil {
		return err
	}
	out, err := h.svc.SetTruth(c.Request().Context(), cc.Campaign.ID, eid, c.Param("rid"), b.Truth, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// beliefBody is the JSON what the party thinks arrives as.
type beliefBody struct {
	Belief string `json:"belief"`
}

// SetBelief records what the party thinks and returns the rumour.
func (h *Handler) SetBelief(c echo.Context) error {
	cc, v, eid, err := h.page(c)
	if err != nil {
		return err
	}
	var b beliefBody
	if err := bind(c, &b); err != nil {
		return err
	}
	out, err := h.svc.SetBelief(c.Request().Context(), cc.Campaign.ID, eid, c.Param("rid"), b.Belief, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Remove deletes a rumour.
func (h *Handler) Remove(c echo.Context) error {
	cc, v, eid, err := h.page(c)
	if err != nil {
		return err
	}
	if err := h.svc.Remove(c.Request().Context(), cc.Campaign.ID, eid, c.Param("rid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
