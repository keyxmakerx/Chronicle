// desk_handler.go serves the desk's fragments: what a line unfolds into, and a
// line's sentence worded again after a change made inside its fold. Both are
// owner-only, like the desk, and both read only the campaign the route names.
// They bind, call the desk service and render; the wording lives in the
// service.

package campaigns

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// deskInputs gathers what campaigns itself knows that the desk draws. A part
// that cannot be read is left out and logged; it never stops the desk or a
// fragment from opening.
func (h *Handler) deskInputs(c *echo.Context, cc *CampaignContext) DeskInputs {
	ctx := c.Request().Context()
	in := DeskInputs{Campaign: cc.Campaign, Addons: map[string]bool{}, GMTerm: layouts.GMTerm(ctx)}
	if members, err := h.service.ListMembers(ctx, cc.Campaign.ID); err != nil {
		slog.Warn("desk: members not read", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
	} else {
		in.Members = members
	}
	if h.invites != nil {
		if inv, err := h.invites.ListInvites(ctx, cc.Campaign.ID); err != nil {
			slog.Warn("desk: invitations not read", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		} else {
			for i := range inv {
				if inv[i].IsPending() {
					in.Invited++
				}
			}
		}
	}
	var addons []PluginHubAddon
	if h.addonLister != nil {
		var err error
		if addons, err = h.addonLister.ListForPluginHub(ctx, cc.Campaign.ID); err != nil {
			slog.Warn("desk: add-ons not read", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
			addons = nil
		}
	}
	for _, a := range addons {
		in.Addons[a.Slug] = a.Enabled
	}
	for _, a := range featureAddons(addons) {
		if !a.Installed {
			continue
		}
		in.FeaturesTotal++
		if a.Enabled {
			in.FeaturesOn++
		}
	}
	if h.systemLister != nil {
		id := currentSystemID(cc.Campaign.ParseSettings().SystemID, addons)
		for _, o := range h.systemLister.ListSystems() {
			if id != "" && o.ID == id {
				in.SystemName = o.Name
			}
		}
	}
	return in
}

// deskOwner returns the campaign context of an owner-only desk request.
func deskOwner(c *echo.Context) (*CampaignContext, error) {
	cc := GetCampaignContext(c)
	if cc == nil {
		return nil, apperror.NewMissingContext()
	}
	if cc.MemberRole < RoleOwner {
		return nil, apperror.NewForbidden("only the campaign owner can open the desk")
	}
	return cc, nil
}

// DeskLineFold renders what a line unfolds into
// (GET /campaigns/:id/dashboard/lines/:line/fold).
func (h *Handler) DeskLineFold(c *echo.Context) error {
	cc, err := deskOwner(c)
	if err != nil {
		return err
	}
	in := h.deskInputs(c, cc)
	req := DeskRequest{CampaignID: cc.Campaign.ID, OwnerID: auth.GetUserID(c), Now: time.Now()}
	fold, ok := h.desk.Fold(c.Request().Context(), req, in, c.Param("line"))
	if !ok {
		return apperror.NewNotFound("that line does not unfold")
	}
	return middleware.Render(c, http.StatusOK, DeskFoldBody(cc, c.Param("line"), fold))
}

// DeskLineSentence renders one line's sentence, worded again from fresh
// numbers (GET /campaigns/:id/dashboard/lines/:line/sentence).
func (h *Handler) DeskLineSentence(c *echo.Context) error {
	cc, err := deskOwner(c)
	if err != nil {
		return err
	}
	in := h.deskInputs(c, cc)
	req := DeskRequest{CampaignID: cc.Campaign.ID, OwnerID: auth.GetUserID(c), Now: time.Now()}
	line, ok := h.desk.Line(c.Request().Context(), req, in, c.Param("line"))
	if !ok {
		return apperror.NewNotFound("no such line")
	}
	return middleware.Render(c, http.StatusOK, deskSentence(line))
}
