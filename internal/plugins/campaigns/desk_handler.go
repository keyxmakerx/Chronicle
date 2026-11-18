// desk_handler.go serves the desk's fragments: what a line unfolds into, and a
// line's sentence worded again after a change made inside its fold. Both are
// owner-only, like the desk, and both read only the campaign the route names.
// They bind, call the desk service and render; the wording lives in the
// service.

package campaigns

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// deskParts says which of campaigns' own facts a caller needs, so a page that
// draws one line does not read what only another line uses.
type deskParts uint8

const (
	deskMembers deskParts = 1 << iota
	deskInvites
	deskAddons
	deskSystem
	deskAll = deskMembers | deskInvites | deskAddons | deskSystem
)

// deskInputs gathers everything the whole desk draws.
func (h *Handler) deskInputs(c *echo.Context, cc *CampaignContext) DeskInputs {
	return h.deskInputsFor(c.Request().Context(), cc.Campaign, deskAll)
}

// deskInputsFor gathers the asked-for facts that campaigns itself knows. A part
// that cannot be read is left out and logged; it never stops the desk or a
// fragment from opening.
func (h *Handler) deskInputsFor(ctx context.Context, campaign *Campaign, parts deskParts) DeskInputs {
	in := DeskInputs{Campaign: campaign, Addons: map[string]bool{}, GMTerm: layouts.GMTerm(ctx)}
	if parts&deskMembers != 0 {
		if members, err := h.service.ListMembers(ctx, campaign.ID); err != nil {
			slog.Warn("desk: members not read", slog.String("campaign_id", campaign.ID), slog.Any("error", err))
		} else {
			in.Members = members
		}
	}
	if parts&deskInvites != 0 && h.invites != nil {
		if inv, err := h.invites.ListInvites(ctx, campaign.ID); err != nil {
			slog.Warn("desk: invitations not read", slog.String("campaign_id", campaign.ID), slog.Any("error", err))
		} else {
			for i := range inv {
				if inv[i].IsPending() {
					in.Invited++
				}
			}
		}
	}
	var addons []PluginHubAddon
	if parts&(deskAddons|deskSystem) != 0 && h.addonLister != nil {
		var err error
		if addons, err = h.addonLister.ListForPluginHub(ctx, campaign.ID); err != nil {
			slog.Warn("desk: add-ons not read", slog.String("campaign_id", campaign.ID), slog.Any("error", err))
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
	if parts&deskSystem != 0 && h.systemLister != nil {
		id := currentSystemID(campaign.ParseSettings().SystemID, addons)
		for _, o := range h.systemLister.ListSystems() {
			if id != "" && o.ID == id {
				in.SystemName = o.Name
			}
		}
	}
	return in
}

// ManageSentence is a Manage page's own desk sentence, for the frame to draw
// under the title (layouts.SetManageSentence). It words the line from the
// same service and providers the desk uses, so the page opened by address and
// the page reached from the desk start with the same words. It returns nil for
// anyone who is not the campaign's owner and when the line cannot be worded;
// the frame then draws nothing.
func (h *Handler) ManageSentence(ctx context.Context, lineID string) templ.Component {
	if !layouts.InCampaign(ctx) || layouts.GetCampaignRole(ctx) < int(RoleOwner) {
		return nil
	}
	campaign, err := h.service.GetByID(ctx, layouts.GetCampaignID(ctx))
	if err != nil || campaign == nil {
		return nil
	}
	parts := deskAddons | deskSystem
	if lineID == "people" {
		parts = deskMembers | deskInvites
	}
	in := h.deskInputsFor(ctx, campaign, parts)
	line, ok := h.desk.Line(ctx, DeskRequest{CampaignID: campaign.ID, OwnerID: layouts.GetUserID(ctx), Now: time.Now()}, in, lineID)
	if !ok {
		return nil
	}
	return manageSentence(lineID, line)
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

// DeskNeedsCount answers how many things wait on the owner (GET
// /campaigns/:id/dashboard/needs), as {"count": N}. The sidebar's Manage row
// asks it after the page has drawn, so no page waits on the providers behind
// it. Only the add-on facts are read, since they alone decide Needs-you.
func (h *Handler) DeskNeedsCount(c *echo.Context) error {
	cc, err := deskOwner(c)
	if err != nil {
		return err
	}
	in := h.deskInputsFor(c.Request().Context(), cc.Campaign, deskAddons)
	req := DeskRequest{CampaignID: cc.Campaign.ID, OwnerID: auth.GetUserID(c), Now: time.Now()}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, map[string]int{"count": h.desk.NeedsCount(c.Request().Context(), req, in)})
}
