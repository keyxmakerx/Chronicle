package systems

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ChoiceRuleHandler serves the hero-choice rules and the picks waiting on a
// Director. It binds, calls the service and renders; the Directors-only rule
// is the service's, with this handler stating who is asking (the owner and
// co-Directors, never a Scribe).
type ChoiceRuleHandler struct {
	svc ChoiceRuleService
}

// NewChoiceRuleHandler creates the handler.
func NewChoiceRuleHandler(svc ChoiceRuleService) *ChoiceRuleHandler {
	return &ChoiceRuleHandler{svc: svc}
}

func noStore(c *echo.Context) { c.Response().Header().Set("Cache-Control", "private, no-store") }

// RulesAPI lists the stored rules.
//
// GET /campaigns/:id/choice-rules?fieldKey=ancestry
func (h *ChoiceRuleHandler) RulesAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	rules, err := h.svc.Rules(c.Request().Context(), cc.Campaign.ID, c.QueryParam("fieldKey"), entryActor(c, cc))
	if err != nil {
		return err
	}
	noStore(c)
	return c.JSON(http.StatusOK, map[string]any{"rules": rules})
}

// SetMarksAPI writes a batch of marks on one pick list and answers with the
// list's rules after.
//
// PUT /campaigns/:id/choice-rules
func (h *ChoiceRuleHandler) SetMarksAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var in ChoiceMarksInput
	if err := readEntryBody(c, &in); err != nil {
		return err
	}
	rules, err := h.svc.SetMarks(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"rules": rules})
}

// CheckMarksAPI is the dry run: the heroes already made that a batch of marks
// would newly break. Nothing is written.
//
// POST /campaigns/:id/choice-rules/check
func (h *ChoiceRuleHandler) CheckMarksAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var in ChoiceMarksInput
	if err := readEntryBody(c, &in); err != nil {
		return err
	}
	res, err := h.svc.CheckMarks(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), in)
	if err != nil {
		return err
	}
	noStore(c)
	return c.JSON(http.StatusOK, res)
}

// EntriesAPI returns a pick list's every entry, the game's and the
// campaign's, with its marks; ?as= shows it as one player gets it.
//
// GET /campaigns/:id/choice-rules/entries?fieldKey=ancestry&as=<userId>
func (h *ChoiceRuleHandler) EntriesAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	list, err := h.svc.DirectorEntries(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), c.QueryParam("fieldKey"), c.QueryParam("as"))
	if err != nil {
		return err
	}
	noStore(c)
	return c.JSON(http.StatusOK, list)
}

// CountAPI says how many rules name a campaign entry, for the delete
// confirmation ("in 2 hero rules").
//
// GET /campaigns/:id/choice-rules/count?entryId=12
func (h *ChoiceRuleHandler) CountAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	id, err := strconv.ParseInt(c.QueryParam("entryId"), 10, 64)
	if err != nil || id <= 0 {
		return apperror.NewBadRequest("invalid entry id")
	}
	n, err := h.svc.RuleCountForEntry(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), id)
	if err != nil {
		return err
	}
	noStore(c)
	return c.JSON(http.StatusOK, map[string]any{"count": n})
}

// RequestsAPI lists the picks waiting on a Director.
//
// GET /campaigns/:id/choice-requests
func (h *ChoiceRuleHandler) RequestsAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	list, err := h.svc.WaitingRequests(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc))
	if err != nil {
		return err
	}
	noStore(c)
	return c.JSON(http.StatusOK, map[string]any{"requests": list, "count": len(list)})
}

func requestID(c *echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("requestId"), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperror.NewBadRequest("invalid request id")
	}
	return id, nil
}

// AllowRequestAPI says yes to a waiting pick: the player may have the entry,
// and the hero's field is set as a Director would.
//
// POST /campaigns/:id/choice-requests/:requestId/allow
func (h *ChoiceRuleHandler) AllowRequestAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	id, err := requestID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.AllowRequest(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), id)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// SuggestRequestAPI closes a waiting pick with a note asking for another.
//
// POST /campaigns/:id/choice-requests/:requestId/suggest  {"message": "..."}
func (h *ChoiceRuleHandler) SuggestRequestAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	id, err := requestID(c)
	if err != nil {
		return err
	}
	var in struct {
		Message string `json:"message"`
	}
	if err := readEntryBody(c, &in); err != nil {
		return err
	}
	if err := h.svc.SuggestAnother(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), id, in.Message); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// RegisterChoiceRuleRoutes mounts the rules and requests API. The groups
// admit any member; every handler's service call refuses anyone but the owner
// and co-Directors, a rule a role middleware cannot express (a Scribe is a
// role above Player yet is not a Director).
func RegisterChoiceRuleRoutes(e *echo.Echo, h *ChoiceRuleHandler, authSvc auth.AuthService, campaignSvc campaigns.CampaignService) {
	rg := e.Group("/campaigns/:id/choice-rules",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		campaigns.RequireRole(campaigns.RolePlayer),
	)
	rg.GET("", h.RulesAPI)
	rg.PUT("", h.SetMarksAPI)
	rg.POST("/check", h.CheckMarksAPI)
	rg.GET("/entries", h.EntriesAPI)
	rg.GET("/count", h.CountAPI)

	qg := e.Group("/campaigns/:id/choice-requests",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		campaigns.RequireRole(campaigns.RolePlayer),
	)
	qg.GET("", h.RequestsAPI)
	qg.POST("/:requestId/allow", h.AllowRequestAPI)
	qg.POST("/:requestId/suggest", h.SuggestRequestAPI)
}
