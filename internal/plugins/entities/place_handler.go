package entities

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SetPlaceService wires the extra-listings service. Without it the "Also
// listed under" line and the tree rows are simply absent.
func (h *Handler) SetPlaceService(svc PlaceService) {
	h.placeSvc = svc
}

// loadPagePlaces reads the line shown under a page's title for this viewer.
// A failure hides the line rather than failing the page.
func (h *Handler) loadPagePlaces(ctx context.Context, cc *campaigns.CampaignContext, entity *Entity, canEdit bool, userID string) pagePlaces {
	if h.placeSvc == nil {
		return pagePlaces{}
	}
	p := pagePlaces{Enabled: true, CanEdit: canEdit}
	if entity.ParentID != nil {
		p.RealParentID = *entity.ParentID
	}
	links, err := h.placeSvc.PlacesOf(ctx, cc.Campaign.ID, entity.ID, int(cc.VisibilityRole()), userID)
	if err != nil {
		slog.Warn("page places: read failed", slog.String("entity_id", entity.ID), slog.Any("error", err))
		return pagePlaces{}
	}
	p.Links = links
	return p
}

// loadTreePlaces reads the listings to draw under the given parents for this
// viewer. Players never see a page their sidebar setting hides.
func (h *Handler) loadTreePlaces(ctx context.Context, cc *campaigns.CampaignContext, parentIDs []string, userID string, hidden map[string]bool) []PlaceLink {
	if h.placeSvc == nil || len(parentIDs) == 0 {
		return nil
	}
	links, err := h.placeSvc.PlacesUnder(ctx, cc.Campaign.ID, parentIDs, int(cc.VisibilityRole()), userID)
	if err != nil {
		slog.Warn("tree places: read failed", slog.Any("error", err))
		return nil
	}
	if cc.MemberRole >= campaigns.RoleScribe || len(hidden) == 0 {
		return links
	}
	out := links[:0]
	for _, l := range links {
		if !hidden[l.EntityID] {
			out = append(out, l)
		}
	}
	return out
}

// editablePage loads the page named in the URL and checks the viewer may edit
// it. The same rule guards add and remove: who can edit the page decides who
// can list it elsewhere.
func (h *Handler) editablePage(c echo.Context, cc *campaigns.CampaignContext) (*Entity, error) {
	entity, err := h.service.GetByID(c.Request().Context(), c.Param("eid"))
	if err != nil || entity.CampaignID != cc.Campaign.ID {
		return nil, apperror.NewNotFound("page not found")
	}
	access, err := h.service.CheckEntityAccess(c.Request().Context(), entity.ID, int(cc.VisibilityRole()), auth.GetUserID(c))
	if err != nil || !access.CanView {
		return nil, apperror.NewNotFound("page not found")
	}
	if !access.CanEdit {
		return nil, apperror.NewForbidden("you can't edit this page")
	}
	return entity, nil
}

// AddPlaceAPI lists a page under one more parent.
// POST /campaigns/:id/entities/:eid/places  {"parent_id": "..."}
// It answers with the refreshed "Also listed under" line.
func (h *Handler) AddPlaceAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.placeSvc == nil {
		return apperror.NewNotFound("page not found")
	}
	entity, err := h.editablePage(c, cc)
	if err != nil {
		return err
	}
	var req struct {
		ParentID string `json:"parent_id"`
	}
	if err := c.Bind(&req); err != nil || req.ParentID == "" {
		return apperror.NewBadRequest("choose a page to list this one under")
	}

	ctx := c.Request().Context()
	userID := auth.GetUserID(c)
	// The parent must be a page this viewer can see. A page they can't see,
	// one in another campaign and one that does not exist all answer alike.
	parent, perr := h.service.GetByID(ctx, req.ParentID)
	if perr != nil || parent.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("page not found")
	}
	pa, perr := h.service.CheckEntityAccess(ctx, parent.ID, int(cc.VisibilityRole()), userID)
	if perr != nil || !pa.CanView {
		return apperror.NewNotFound("page not found")
	}

	if err := h.placeSvc.AddPlace(ctx, cc.Campaign.ID, entity.ID, parent.ID, userID); err != nil {
		return err
	}
	h.logAudit(c, cc.Campaign.ID, "entity.place_added", entity.ID, entity.Name)
	return h.renderPlaces(c, cc, entity)
}

// RemovePlaceAPI takes a page out of one extra place. The page is untouched.
// DELETE /campaigns/:id/entities/:eid/places/:pid
func (h *Handler) RemovePlaceAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.placeSvc == nil {
		return apperror.NewNotFound("page not found")
	}
	entity, err := h.editablePage(c, cc)
	if err != nil {
		return err
	}
	if err := h.placeSvc.RemovePlace(c.Request().Context(), cc.Campaign.ID, entity.ID, c.Param("pid")); err != nil {
		return err
	}
	h.logAudit(c, cc.Campaign.ID, "entity.place_removed", entity.ID, entity.Name)
	return h.renderPlaces(c, cc, entity)
}

// renderPlaces answers with the swappable line for the page.
func (h *Handler) renderPlaces(c echo.Context, cc *campaigns.CampaignContext, entity *Entity) error {
	p := h.loadPagePlaces(c.Request().Context(), cc, entity, true, auth.GetUserID(c))
	return middleware.Render(c, http.StatusOK, entityPlaces(cc, entity, p))
}
