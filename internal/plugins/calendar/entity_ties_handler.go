// entity_ties_handler.go — production event<->entity tie endpoints
// (C-CAL-WORLDSTATE-PRODUCTION-PORT 2b). The event editor's attach-entity
// picker (the production form of Phase 1.5's mock picker) reads/writes these
// to persist REAL entity_event_links (#402) with a participation_role.
//
// Cross-plugin stays clean: the picker SEARCHES entities via the entities
// plugin's own GET /entities/search (browser-side); these endpoints only
// touch the calendar service's tie methods. No calendar→entities Go import.
//
// Permissions mirror the event model: read = Player+, write = Scribe+. IDOR
// is closed via requireEventInCampaign (the event's calendar must belong to
// the campaign in the path).
package calendar

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// apiEntityTie is the wire shape for one attached entity (the picker chip).
type apiEntityTie struct {
	EntityID          string  `json:"entity_id"`
	EntityName        string  `json:"entity_name"`
	EntityType        string  `json:"entity_type"`
	EntityIcon        string  `json:"entity_icon"`
	EntityColor       string  `json:"entity_color"`
	ParticipationRole *string `json:"participation_role,omitempty"`
}

// ListEventEntitiesAPI — GET /campaigns/:id/calendars/:calId/events/:eid/entities.
// Returns the entities tied to an event (with role) for the picker. Player+.
func (h *Handler) ListEventEntitiesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	eventID := c.Param("eid")

	if _, err := h.requireEventInCampaign(c, eventID, cc.Campaign.ID); err != nil {
		return err
	}
	ties, err := h.svc.EntitiesForEvent(ctx, eventID)
	if err != nil {
		return err
	}
	out := make([]apiEntityTie, 0, len(ties))
	for _, t := range ties {
		out = append(out, apiEntityTie(t))
	}
	return c.JSON(http.StatusOK, out)
}

// LinkEventEntityAPI — PUT /campaigns/:id/calendars/:calId/events/:eid/entities/:entityId.
// Attaches an entity to the event with a participation role (idempotent —
// re-linking updates the role). Body: {"role":"involved|present|affected|mentioned"}.
// Empty role defaults to "involved". Scribe+.
func (h *Handler) LinkEventEntityAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	eventID := c.Param("eid")
	entityID := c.Param("entityId")
	if entityID == "" {
		return apperror.NewBadRequest("entityId is required")
	}
	if _, err := h.requireEventInCampaign(c, eventID, cc.Campaign.ID); err != nil {
		return err
	}
	var req struct {
		Role string `json:"role"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	// Service validates the role against the four pinned ParticipationRoles.
	if err := h.svc.LinkEntityToEvent(ctx, entityID, eventID, req.Role); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "entity_id": entityID})
}

// UnlinkEventEntityAPI — DELETE /campaigns/:id/calendars/:calId/events/:eid/entities/:entityId.
// Detaches an entity from the event. Idempotent (no-op if absent). Scribe+.
func (h *Handler) UnlinkEventEntityAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	eventID := c.Param("eid")
	entityID := c.Param("entityId")
	if entityID == "" {
		return apperror.NewBadRequest("entityId is required")
	}
	if _, err := h.requireEventInCampaign(c, eventID, cc.Campaign.ID); err != nil {
		return err
	}
	if err := h.svc.UnlinkEntityFromEvent(ctx, entityID, eventID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
