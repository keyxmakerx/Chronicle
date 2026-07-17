// entity_actions.go — the event editor's "create entity from event" action
// (C-CAL-EDITOR-EXPANSION PR1, item 1). Clicking the action in the V2 drawer
// creates a campaign entity named after the event and links it to the event via
// the existing entity↔event tie, then the toast offers a link to the new entity.
//
// Plugin isolation (CLAUDE.md rule 8): the calendar plugin never imports the
// entities repo or writes its tables. It declares the narrow EntityCreator
// interface it needs; the app wiring (internal/app/routes.go) supplies an
// adapter over the entities service. This mirrors the bestiary plugin's
// EntityCreator seam and calendar's own TimelineLister pattern.
package calendar

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EntityCreator is the cross-plugin seam the "create entity from event" action
// needs from the entities plugin. Authorization is the entities plugin's own
// creation policy; the calendar route additionally gates Scribe+.
type EntityCreator interface {
	// ListEntityTypes returns the campaign's entity types for the picker.
	ListEntityTypes(ctx context.Context, campaignID string) ([]EntityTypeRef, error)
	// CreateEntity creates an entity of typeID named `name` in the campaign,
	// authored by userID, and returns its id.
	CreateEntity(ctx context.Context, campaignID, userID string, typeID int, name string) (entityID string, err error)
}

// EntityTypeRef is the calendar plugin's minimal view of an entity type for the
// create-entity picker — avoids importing the entities model (rule 8).
type EntityTypeRef struct {
	ID   int
	Name string
	Slug string
}

// SetEntityCreator wires the cross-plugin entity-creation seam. Optional: when
// unset, the "create entity from event" action is omitted (no entity types) and
// the endpoint returns an error.
func (h *Handler) SetEntityCreator(ec EntityCreator) { h.entityCreator = ec }

// CreateEntityFromEventAPI — POST /campaigns/:id/calendars/:calId/events/:eid/create-entity.
// Body: {"entity_type_id": N}. Creates an entity named after the event, links it
// to the event, and returns the new entity's id + page URL for the result toast.
// Scribe+ (route-gated); IDOR closed via requireEventInCampaign.
func (h *Handler) CreateEntityFromEventAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	eventID := c.Param("eid")

	evt, err := h.requireEventInCampaign(c, eventID, cc.Campaign.ID)
	if err != nil {
		return err
	}
	if h.entityCreator == nil {
		return apperror.NewInternal(fmt.Errorf("entity creation seam not configured"))
	}

	var req struct {
		EntityTypeID int `json:"entity_type_id"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if req.EntityTypeID <= 0 {
		return apperror.NewBadRequest("entity_type_id is required")
	}

	entityID, err := h.svc.CreateEntityFromEvent(ctx, h.entityCreator, cc.Campaign.ID,
		auth.GetUserID(c), req.EntityTypeID, evt.Name, eventID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{
		"entity_id": entityID,
		"name":      evt.Name,
		"edit_url":  "/campaigns/" + cc.Campaign.ID + "/entities/" + entityID,
	})
}
