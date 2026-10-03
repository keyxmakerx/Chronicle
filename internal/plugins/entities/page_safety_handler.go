package entities

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// PageSafetyHandler serves a page's History panel and the campaign Trash.
// History is for owners and scribes (the people who can edit pages); the
// Trash is the owner's, like deleting.
type PageSafetyHandler struct {
	svc      PageSafetyService
	entities EntityService
	members  MemberLister
}

// NewPageSafetyHandler creates the History/Trash handler. members may be nil;
// history then shows names without roles.
func NewPageSafetyHandler(svc PageSafetyService, entities EntityService, members MemberLister) *PageSafetyHandler {
	return &PageSafetyHandler{svc: svc, entities: entities, members: members}
}

// RegisterPageSafetyRoutes mounts History and Trash.
func RegisterPageSafetyRoutes(e *echo.Echo, h *PageSafetyHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		withActorMiddleware,
	)
	scribe := campaigns.RequireRole(campaigns.RoleScribe)
	owner := campaigns.RequireRole(campaigns.RoleOwner)
	cg.GET("/entities/:eid/history", h.History, scribe)
	cg.POST("/entities/:eid/history/:vid/restore", h.RestoreVersion, scribe)
	cg.GET("/trash", h.Trash, owner)
	cg.POST("/trash/:eid/restore", h.RestoreFromTrash, owner)
}

// pageInCampaign loads a live page the caller may see, so an id from another
// campaign, or a page hidden from this scribe, reads as not found. History
// holds every past text of the page, so it is gated like the page itself;
// edit says the caller must also be allowed to change it.
func (h *PageSafetyHandler) pageInCampaign(c echo.Context, cc *campaigns.CampaignContext, edit bool) (*Entity, error) {
	ctx := c.Request().Context()
	entity, err := h.entities.GetByID(ctx, c.Param("eid"))
	if err != nil {
		return nil, err
	}
	if entity.CampaignID != cc.Campaign.ID {
		return nil, apperror.NewNotFound("entity not found")
	}
	// VisibilityRole, as on the page itself, so a Co-DM sees what the owner sees.
	access, err := h.entities.CheckEntityAccess(ctx, entity.ID, int(cc.VisibilityRole()), auth.GetUserID(c))
	if err != nil || !access.CanView {
		return nil, apperror.NewNotFound("entity not found")
	}
	if edit && !access.CanEdit {
		return nil, apperror.NewForbidden("you can't edit this page")
	}
	return entity, nil
}

// History renders the History panel (GET .../entities/:eid/history?v=<id>).
// v picks the version whose changes are shown; the newest by default.
func (h *PageSafetyHandler) History(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	entity, err := h.pageInCampaign(c, cc, false)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	versions, selected, err := h.svc.History(ctx, entity.ID, c.QueryParam("v"))
	if err != nil {
		return err
	}
	data := HistoryPanelData{
		CampaignID: cc.Campaign.ID,
		Entity:     entity,
		Versions:   versions,
		Selected:   selected,
		ViewerID:   auth.GetUserID(c),
		Roles:      h.roleLabels(c, cc.Campaign.ID),
		Now:        time.Now(),
	}
	return middleware.Render(c, http.StatusOK, HistoryPanel(data))
}

// roleLabels maps member user ids to their role word ("scribe"), for the
// "Sam (scribe)" labels. Best-effort: a failed read just drops the roles.
func (h *PageSafetyHandler) roleLabels(c echo.Context, campaignID string) map[string]string {
	out := map[string]string{}
	if h.members == nil {
		return out
	}
	members, err := h.members.ListMembers(c.Request().Context(), campaignID)
	if err != nil {
		return out
	}
	for _, m := range members {
		out[m.UserID] = m.Role.String()
	}
	return out
}

// RestoreVersion puts an old version back and reloads the page
// (POST .../entities/:eid/history/:vid/restore).
func (h *PageSafetyHandler) RestoreVersion(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	entity, err := h.pageInCampaign(c, cc, true)
	if err != nil {
		return err
	}
	if err := h.svc.RestoreVersion(c.Request().Context(), entity.ID, c.Param("vid")); err != nil {
		return err
	}
	return middleware.HTMXRedirect(c, "/campaigns/"+cc.Campaign.ID+"/entities/"+entity.ID)
}

// Trash renders the campaign's Trash under Manage (GET /campaigns/:id/trash).
func (h *PageSafetyHandler) Trash(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	items, err := h.svc.ListTrash(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}
	data := TrashPageData{
		CampaignID: cc.Campaign.ID,
		Items:      items,
		Retention:  h.svc.RetentionDays(ctx),
		ViewerID:   auth.GetUserID(c),
		Now:        time.Now(),
		Restored:   c.QueryParam("restored"),
	}
	return middleware.Render(c, http.StatusOK, TrashPage(cc, data))
}

// RestoreFromTrash brings a page back (POST /campaigns/:id/trash/:eid/restore).
// The Undo toast calls it with fetch and gets JSON; the Trash page's button
// is an htmx form and gets the refreshed Trash.
func (h *PageSafetyHandler) RestoreFromTrash(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	item, err := h.svc.RestoreFromTrash(c.Request().Context(), cc.Campaign.ID, c.Param("eid"))
	if err != nil {
		return err
	}
	pageURL := "/campaigns/" + cc.Campaign.ID + "/entities/" + item.ID
	if middleware.IsHTMX(c) {
		return middleware.HTMXRedirect(c, "/campaigns/"+cc.Campaign.ID+"/trash?restored="+item.ID)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": "ok", "id": item.ID, "name": item.Name, "url": pageURL})
}
