package entities

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SetPageLayouts wires the named layouts service; without it every category
// has only its main layout.
func (h *Handler) SetPageLayouts(s PageLayoutService) { h.pageLayouts = s }

type wornLayoutKey struct{}

// wornLayoutView is what the page header shows about the layout a page wears.
type wornLayoutView struct {
	Info  PageLayoutInfo
	Named bool // a named layout, not the category's main one
}

// wornLayout returns the category to draw the page with (a copy carrying the
// named layout's blocks when the page wears one) and a context that records
// which layout that is. A category with no named layouts is returned as is.
func (h *Handler) wornLayout(ctx context.Context, cc *campaigns.CampaignContext, e *Entity, et *EntityType) (*EntityType, context.Context) {
	if h.pageLayouts == nil || et == nil {
		return et, ctx
	}
	pl, key := h.pageLayouts.Worn(ctx, e, et)
	if pl.Empty() {
		return et, ctx
	}
	view := wornLayoutView{
		Info:  PageLayoutInfo{Key: key, Name: pl.Label(et, key), Choices: pl.choices(et)},
		Named: key != MainLayoutKey,
	}
	if view.Named {
		cp := *et
		cp.Layout = pl.LayoutFor(et, key)
		et = &cp
	}
	return et, context.WithValue(ctx, wornLayoutKey{}, view)
}

// wornLayoutFrom returns what wornLayout recorded; ok is false for a page
// whose category has only its main layout.
func wornLayoutFrom(ctx context.Context) (wornLayoutView, bool) {
	v, ok := ctx.Value(wornLayoutKey{}).(wornLayoutView)
	return v, ok
}

// wearsNamedLayout reports whether the page being drawn wears one of its
// category's named layouts. Such a page is drawn from that layout even where
// a game system would otherwise draw the whole page; the system's sheet is a
// block the layout can place.
func wearsNamedLayout(ctx context.Context) bool {
	v, ok := wornLayoutFrom(ctx)
	return ok && v.Named
}

type typeLayoutsKey struct{}

// withTypeLayouts records a category's named layouts for the Customizer's
// page layout tab; a context without them reads as named layouts being off.
func withTypeLayouts(ctx context.Context, p PageLayouts) context.Context {
	return context.WithValue(ctx, typeLayoutsKey{}, p)
}

// typeLayoutsFrom returns what withTypeLayouts recorded.
func typeLayoutsFrom(ctx context.Context) (PageLayouts, bool) {
	p, ok := ctx.Value(typeLayoutsKey{}).(PageLayouts)
	return p, ok
}

// canPickPageLayout reports whether the viewer may change which layout a page
// wears: the Director team, not while viewing as a player.
func canPickPageLayout(cc *campaigns.CampaignContext) bool {
	return cc != nil && (cc.MemberRole >= campaigns.RoleOwner || cc.IsDmGranted)
}

// SetPageLayoutAPI makes a page wear one of its category's layouts.
// PUT /campaigns/:id/entities/:eid/layout  body: {"layout": "<key>"}
func (h *Handler) SetPageLayoutAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.pageLayouts == nil {
		return apperror.NewNotFound("this campaign's categories have one layout each")
	}
	if !canPickPageLayout(cc) || campaigns.ViewingAsPlayer(c, cc) {
		return apperror.NewForbidden("only the Director team can change a page's layout")
	}
	var body struct {
		Layout string `json:"layout" form:"layout"`
	}
	if err := c.Bind(&body); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	ctx := c.Request().Context()
	e, err := h.service.GetByID(ctx, c.Param("eid"))
	if err != nil || e.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("page not found")
	}
	et, err := h.service.GetEntityTypeByID(ctx, e.EntityTypeID)
	if err != nil {
		return apperror.NewNotFound("page not found")
	}
	pl := h.pageLayouts.ForType(ctx, et)
	key := strings.TrimSpace(body.Layout)
	if k, ok := pl.KeyByName(et, key); ok {
		key = k
	}
	if err := h.pageLayouts.SetPageLayout(ctx, e, et, key); err != nil {
		return err
	}
	h.logAuditWithDetails(c, cc.Campaign.ID, "entity.layout_changed", e.ID, e.Name, map[string]any{"layout": pl.Label(et, key)})
	if c.Request().Header.Get("HX-Request") != "" {
		c.Response().Header().Set("HX-Refresh", "true")
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, map[string]string{"layout": key, "name": pl.Label(et, key)})
}

// pageLayoutURL is where the chip on a page sends a new choice.
func pageLayoutURL(cc *campaigns.CampaignContext, e *Entity) string {
	return fmt.Sprintf("/campaigns/%s/entities/%s/layout", cc.Campaign.ID, e.ID)
}
