// desk_handler.go gathers the desk's inputs and serves its owner-only
// answers: the figures a Manage tool page draws beside its title, the count
// behind the sidebar's Manage row, and Find's list. Each reads only the
// campaign the route names. They bind, call the desk service and render; the
// wording lives in the service.

package campaigns

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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

// deskInputs gathers everything the whole desk draws, the Customize block's
// words included: the look, the categories, what each plugin's Customize
// area says is chosen now, and this viewer's own brush choice.
func (h *Handler) deskInputs(c *echo.Context, cc *CampaignContext) DeskInputs {
	ctx := c.Request().Context()
	in := h.deskInputsFor(ctx, cc.Campaign, deskAll)
	in.Look = lookSummaryOf(ctx, cc.Campaign)
	if h.categoryLister != nil {
		if cats, err := h.categoryLister.ListCustomizeCategories(ctx, cc.Campaign.ID); err != nil {
			slog.Warn("desk: categories not read", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		} else {
			in.Categories, in.CategoriesRead = cats, true
		}
	}
	for _, t := range h.customizeTabsOn(cc, in.Addons) {
		in.Tabs = append(in.Tabs, DeskTab{ID: t.ID, Label: t.Label, Summary: t.summary(ctx)})
	}
	in.BrushHidden = h.brushHidden(ctx, auth.GetUserID(c), cc.Campaign.ID)
	return in
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
			in.InvitedRead = true
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
		in.SystemID = id
		for _, o := range h.systemLister.ListSystems() {
			in.Systems = append(in.Systems, DeskSystem{ID: o.ID, Name: o.Name})
			if id != "" && o.ID == id {
				in.SystemName = o.Name
			}
		}
	}
	return in
}

// ManageFigure is a Manage page's own figures, for the frame to draw beside
// the title (layouts.SetManageFigure). It builds them from the same service
// and providers the desk uses, so the page opened by address and the page
// reached from the desk show the same figures. It returns nil for anyone who
// is not the campaign's owner and when the figures cannot be read; the frame
// then draws nothing.
func (h *Handler) ManageFigure(ctx context.Context, lineID string) templ.Component {
	fig, ok := h.manageFigureOf(ctx, lineID)
	if !ok || fig.Empty() {
		return nil
	}
	return manageFigure(lineID, fig)
}

// manageFigureSwap is the same figure as an out-of-band swap, for a response
// that changes what the figure counts (a removal, an invite). It is built when
// the response is drawn, because the campaign and the owner are only in the
// render context, not the request's. A figure that can no longer be read
// swaps in an empty one rather than leaving a stale count.
func (h *Handler) manageFigureSwap(lineID string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		fig, ok := h.manageFigureOf(ctx, lineID)
		if !ok {
			return nil
		}
		return manageFigureSwap(lineID, fig).Render(ctx, w)
	})
}

func (h *Handler) manageFigureOf(ctx context.Context, lineID string) (DeskFigure, bool) {
	if !layouts.InCampaign(ctx) || layouts.GetCampaignRole(ctx) < int(RoleOwner) {
		return DeskFigure{}, false
	}
	campaign, err := h.service.GetByID(ctx, layouts.GetCampaignID(ctx))
	if err != nil || campaign == nil {
		return DeskFigure{}, false
	}
	parts := deskAddons | deskSystem
	if lineID == "people" {
		parts = deskMembers | deskInvites
	}
	in := h.deskInputsFor(ctx, campaign, parts)
	return h.desk.Figure(ctx, DeskRequest{CampaignID: campaign.ID, OwnerID: layouts.GetUserID(ctx), Now: time.Now()}, in, lineID)
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
