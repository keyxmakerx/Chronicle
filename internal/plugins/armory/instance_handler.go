// instance_handler.go provides HTTP endpoints for inventory instance CRUD.
// Thin handlers that bind request, call service, and return HTMX or JSON.
package armory

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// InstanceHandler serves inventory instance endpoints.
type InstanceHandler struct {
	svc InstanceService
}

// NewInstanceHandler creates a new instance handler.
func NewInstanceHandler(svc InstanceService) *InstanceHandler {
	return &InstanceHandler{svc: svc}
}

// ListInstances returns all instances for a campaign (GET /campaigns/:id/armory/instances).
func (h *InstanceHandler) ListInstances(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	instances, err := h.svc.ListInstances(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return apperror.NewInternal(err)
	}

	return c.JSON(http.StatusOK, instances)
}

// CreateInstance creates a new inventory instance (POST /campaigns/:id/armory/instances).
func (h *InstanceHandler) CreateInstance(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var input CreateInstanceInput

	// Support both JSON and form data.
	ct := c.Request().Header.Get("Content-Type")
	if ct == "application/json" || ct == "application/json; charset=utf-8" {
		if err := c.Bind(&input); err != nil {
			return apperror.NewBadRequest("invalid request body")
		}
	} else {
		input.Name = c.FormValue("name")
		input.Description = c.FormValue("description")
		input.Icon = c.FormValue("icon")
		input.Color = c.FormValue("color")
	}

	inst, err := h.svc.CreateInstance(c.Request().Context(), cc.Campaign.ID, input)
	if err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		// Reload the armory page to show the new instance.
		c.Response().Header().Set("HX-Redirect", "/campaigns/"+cc.Campaign.ID+"/armory?instance="+strconv.Itoa(inst.ID))
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusCreated, inst)
}

// UpdateInstance modifies an instance (PUT /campaigns/:id/armory/instances/:iid).
func (h *InstanceHandler) UpdateInstance(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	instanceID, err := strconv.Atoi(c.Param("iid"))
	if err != nil {
		return apperror.NewBadRequest("invalid instance ID")
	}

	var input CreateInstanceInput
	ct := c.Request().Header.Get("Content-Type")
	if ct == "application/json" || ct == "application/json; charset=utf-8" {
		if err := c.Bind(&input); err != nil {
			return apperror.NewBadRequest("invalid request body")
		}
	} else {
		input.Name = c.FormValue("name")
		input.Description = c.FormValue("description")
		input.Icon = c.FormValue("icon")
		input.Color = c.FormValue("color")
	}

	if err := h.svc.UpdateInstance(c.Request().Context(), cc.Campaign.ID, instanceID, input); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/campaigns/"+cc.Campaign.ID+"/armory?instance="+strconv.Itoa(instanceID))
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteInstance removes an instance (DELETE /campaigns/:id/armory/instances/:iid).
func (h *InstanceHandler) DeleteInstance(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	instanceID, err := strconv.Atoi(c.Param("iid"))
	if err != nil {
		return apperror.NewBadRequest("invalid instance ID")
	}

	if err := h.svc.DeleteInstance(c.Request().Context(), cc.Campaign.ID, instanceID); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/campaigns/"+cc.Campaign.ID+"/armory")
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// AddItem adds an entity to an instance (POST /campaigns/:id/armory/instances/:iid/items).
func (h *InstanceHandler) AddItem(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	instanceID, err := strconv.Atoi(c.Param("iid"))
	if err != nil {
		return apperror.NewBadRequest("invalid instance ID")
	}

	var body struct {
		EntityID string `json:"entity_id"`
	}
	ct := c.Request().Header.Get("Content-Type")
	if ct == "application/json" || ct == "application/json; charset=utf-8" {
		if err := c.Bind(&body); err != nil {
			return apperror.NewBadRequest("invalid request body")
		}
	} else {
		body.EntityID = c.FormValue("entity_id")
	}

	if err := h.svc.AddItem(c.Request().Context(), cc.Campaign.ID, instanceID, body.EntityID); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Trigger", "instance-items-changed")
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// RemoveItem removes an entity from an instance (DELETE /campaigns/:id/armory/instances/:iid/items/:eid).
func (h *InstanceHandler) RemoveItem(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	instanceID, err := strconv.Atoi(c.Param("iid"))
	if err != nil {
		return apperror.NewBadRequest("invalid instance ID")
	}

	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	if err := h.svc.RemoveItem(c.Request().Context(), cc.Campaign.ID, instanceID, entityID); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Trigger", "instance-items-changed")
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
