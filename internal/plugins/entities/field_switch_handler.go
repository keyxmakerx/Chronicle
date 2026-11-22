package entities

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SetFieldSwitch wires the game-system field switch. Unwired, no offer is
// drawn and the switch routes answer 404.
func (h *Handler) SetFieldSwitch(svc *FieldSwitchService) {
	h.fieldSwitch = svc
}

type fieldSwitchViewKey struct{}

// withFieldSwitchView carries the owner's view of a category's offer to the
// templates that draw it (the category list, the category's Fields tab).
func withFieldSwitchView(ctx context.Context, v *FieldSwitchView) context.Context {
	if v == nil {
		return ctx
	}
	return context.WithValue(ctx, fieldSwitchViewKey{}, v)
}

func fieldSwitchViewOf(ctx context.Context) *FieldSwitchView {
	v, _ := ctx.Value(fieldSwitchViewKey{}).(*FieldSwitchView)
	return v
}

// addFieldSwitchView puts the category's offer on the request for its owner.
// Only the owner answers it, so nobody else's page carries it.
func (h *Handler) addFieldSwitchView(c *echo.Context, cc *campaigns.CampaignContext, et *EntityType) {
	if h.fieldSwitch == nil || cc == nil || et == nil || cc.MemberRole < campaigns.RoleOwner {
		return
	}
	if v := h.fieldSwitch.View(c.Request().Context(), cc.Campaign.ID, et); v != nil {
		c.SetRequest(c.Request().WithContext(withFieldSwitchView(c.Request().Context(), v)))
	}
}

// fieldSwitchReturn is where a switch form sends the owner back to: the
// category's list, or its Fields tab in the Customizer. Fixed paths, never a
// URL from the request.
func fieldSwitchReturn(c *echo.Context, campaignID string, typeID int) string {
	if c.FormValue("return_to") == "fields" {
		return fmt.Sprintf("/campaigns/%s/customize/categories/%d/fields", campaignID, typeID)
	}
	return fmt.Sprintf("/campaigns/%s/entities?type=%d", campaignID, typeID)
}

// fieldSwitchTarget reads the route's category id.
func (h *Handler) fieldSwitchTarget(c *echo.Context) (*campaigns.CampaignContext, int, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return nil, 0, apperror.NewMissingContext()
	}
	if h.fieldSwitch == nil {
		return nil, 0, apperror.NewNotFound("not found")
	}
	id, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return nil, 0, apperror.NewBadRequest("invalid entity type ID")
	}
	return cc, id, nil
}

// AcceptFieldSwitch puts the game system's fields on the category.
// POST /campaigns/:id/entity-types/:etid/field-switch
func (h *Handler) AcceptFieldSwitch(c *echo.Context) error {
	cc, id, err := h.fieldSwitchTarget(c)
	if err != nil {
		return err
	}
	if _, err := h.fieldSwitch.Accept(c.Request().Context(), cc.Campaign.ID, id, auth.GetUserID(c)); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, fieldSwitchReturn(c, cc.Campaign.ID, id))
}

// SwitchFieldsBack puts the category's old fields and kept values back.
// POST /campaigns/:id/entity-types/:etid/field-switch/back
func (h *Handler) SwitchFieldsBack(c *echo.Context) error {
	cc, id, err := h.fieldSwitchTarget(c)
	if err != nil {
		return err
	}
	if _, err := h.fieldSwitch.Back(c.Request().Context(), cc.Campaign.ID, id, auth.GetUserID(c)); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, fieldSwitchReturn(c, cc.Campaign.ID, id))
}

// DismissFieldSwitch is "Not now": the list stops offering the switch.
// POST /campaigns/:id/entity-types/:etid/field-switch/dismiss
func (h *Handler) DismissFieldSwitch(c *echo.Context) error {
	cc, id, err := h.fieldSwitchTarget(c)
	if err != nil {
		return err
	}
	if err := h.fieldSwitch.Dismiss(c.Request().Context(), cc.Campaign.ID, id, auth.GetUserID(c)); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, fieldSwitchReturn(c, cc.Campaign.ID, id))
}
