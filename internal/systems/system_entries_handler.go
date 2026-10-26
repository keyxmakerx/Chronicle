package systems

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// maxSystemEntryBody bounds a request: the description cap plus room for the
// other fields and JSON escaping.
const maxSystemEntryBody = 128 * 1024

// SystemEntryHandler serves a campaign's own pick-list entries as JSON. It
// only binds, calls the service and renders; the Director rule is enforced by
// the service, with this handler stating who is asking.
type SystemEntryHandler struct {
	svc SystemEntryService
}

// NewSystemEntryHandler creates the handler.
func NewSystemEntryHandler(svc SystemEntryService) *SystemEntryHandler {
	return &SystemEntryHandler{svc: svc}
}

func entryActor(c echo.Context, cc *campaigns.CampaignContext) EntryActor {
	return EntryActor{UserID: auth.GetUserID(c), IsDirector: cc.CanAuthorDmOnly()}
}

func readEntryBody(c echo.Context, dst any) error {
	data, err := io.ReadAll(io.LimitReader(c.Request().Body, maxSystemEntryBody+1))
	if err != nil || len(data) > maxSystemEntryBody {
		return apperror.NewBadRequest("the request is empty or too large")
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return apperror.NewBadRequest("the request is not valid JSON")
	}
	return nil
}

func entryID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("entryId"), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperror.NewBadRequest("invalid entry id")
	}
	return id, nil
}

// ListAPI returns the campaign's entries; a Director also sees the
// Directors-only ones.
//
// GET /campaigns/:id/system-entries?fieldKey=ancestry
func (h *SystemEntryHandler) ListAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	list, err := h.svc.List(c.Request().Context(), cc.Campaign.ID, c.QueryParam("fieldKey"), entryActor(c, cc))
	if err != nil {
		return err
	}
	// Private and uncached: what comes back depends on the viewer's role.
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, map[string]any{"entries": list})
}

// CreateAPI adds an entry.
//
// POST /campaigns/:id/system-entries
func (h *SystemEntryHandler) CreateAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var in CreateSystemEntryInput
	if err := readEntryBody(c, &in); err != nil {
		return err
	}
	e, err := h.svc.Create(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, map[string]any{"entry": e})
}

// UpdateAPI changes an entry. The body is a partial update: absent keys keep
// their value, null clears, a value replaces.
//
// PUT /campaigns/:id/system-entries/:entryId
func (h *SystemEntryHandler) UpdateAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	id, err := entryID(c)
	if err != nil {
		return err
	}
	var in UpdateSystemEntryInput
	if err := readEntryBody(c, &in); err != nil {
		return err
	}
	e, err := h.svc.Update(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), id, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"entry": e})
}

// DeleteAPI removes an entry. Characters that picked it keep the name they
// stored.
//
// DELETE /campaigns/:id/system-entries/:entryId
func (h *SystemEntryHandler) DeleteAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	id, err := entryID(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), id); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// GrantsAPI lists the permissions to write an entry: every one for a
// Director, the caller's own for a player.
//
// GET /campaigns/:id/system-entries/grants
func (h *SystemEntryHandler) GrantsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	list, err := h.svc.Grants(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc))
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, map[string]any{"grants": list})
}

// GrantAPI lets a player make their own entry for a pick list, or renews
// the permission.
//
// POST /campaigns/:id/system-entries/grants
func (h *SystemEntryHandler) GrantAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var in GrantInput
	if err := readEntryBody(c, &in); err != nil {
		return err
	}
	g, err := h.svc.Grant(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"grant": g})
}

// RevokeGrantAPI takes a permission back.
//
// DELETE /campaigns/:id/system-entries/grants/:grantId
func (h *SystemEntryHandler) RevokeGrantAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	id, err := strconv.ParseInt(c.Param("grantId"), 10, 64)
	if err != nil || id <= 0 {
		return apperror.NewBadRequest("invalid permission id")
	}
	if err := h.svc.RevokeGrant(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), id); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// RegisterSystemEntryRoutes mounts the entry API. The group admits any
// member (they may read their own view of the list); writes are refused by
// the service unless the caller is the owner or a co-Director, or a player
// writing the one entry a Director let them make, rules a role middleware
// cannot express.
func RegisterSystemEntryRoutes(e *echo.Echo, h *SystemEntryHandler, authSvc auth.AuthService, campaignSvc campaigns.CampaignService) {
	g := e.Group("/campaigns/:id/system-entries",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		campaigns.RequireRole(campaigns.RolePlayer),
	)
	g.GET("", h.ListAPI)
	g.GET("/grants", h.GrantsAPI)
	g.POST("/grants", h.GrantAPI)
	g.DELETE("/grants/:grantId", h.RevokeGrantAPI)
	g.POST("", h.CreateAPI)
	g.PUT("/:entryId", h.UpdateAPI)
	g.DELETE("/:entryId", h.DeleteAPI)
}
