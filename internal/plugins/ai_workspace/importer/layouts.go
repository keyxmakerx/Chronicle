package importer

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// LayoutAccess is the part of the entities page layout service that
// `layout:` on a page needs; entities.PageLayoutService satisfies it.
type LayoutAccess interface {
	ForType(ctx context.Context, et *entities.EntityType) entities.PageLayouts
	SetPageLayout(ctx context.Context, e *entities.Entity, et *entities.EntityType, key string) error
}

// SetLayouts wires `layout:` on page rows. Optional: unwired, a row that
// names a layout is refused rather than saved wearing the wrong one.
func (c *Committer) SetLayouts(l LayoutAccess) { c.layouts = l }

// resolveLayout finds the layout a row names among its category's, before
// anything is written, so a name that matches none refuses the row. It
// returns "" when the row names no layout.
func (c *Committer) resolveLayout(ctx context.Context, campaignID string, typeID int, page ParsedPage) (string, string) {
	name := strings.TrimSpace(page.FrontMatter.Layout)
	if name == "" {
		return "", ""
	}
	if c.layouts == nil {
		return "", "This server can't set a page's layout, so this page wasn't saved. Take out `layout:` and save it again."
	}
	et := c.typeByID(ctx, campaignID, typeID)
	if et == nil {
		return "", "Couldn't read this page's category to find its layout. Try again in a moment."
	}
	pl := c.layouts.ForType(ctx, et)
	if key, ok := pl.KeyByName(et, name); ok {
		return key, ""
	}
	names := []string{pl.MainLabel(et)}
	for _, l := range pl.Extra {
		names = append(names, l.Name)
	}
	return "", fmt.Sprintf("%s has no layout called %q. Its layouts: %s.", et.NamePlural, name, strings.Join(names, ", "))
}

// applyLayout makes the saved page, in the category the row resolved, wear
// the layout resolveLayout found.
func (c *Committer) applyLayout(ctx context.Context, campaignID string, typeID int, e *entities.Entity, key string) string {
	if key == "" || c.layouts == nil || e == nil {
		return ""
	}
	page := *e
	page.CampaignID, page.EntityTypeID = campaignID, typeID
	et := c.typeByID(ctx, campaignID, typeID)
	if et == nil || c.layouts.SetPageLayout(ctx, &page, et, key) != nil {
		return "Saved the page, but couldn't set its layout. Pick it from the chip beside the page's name."
	}
	return ""
}

// typeByID is one of the campaign's categories, or nil.
func (c *Committer) typeByID(ctx context.Context, campaignID string, typeID int) *entities.EntityType {
	types, err := c.creator.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil
	}
	for i := range types {
		if types[i].ID == typeID {
			return &types[i]
		}
	}
	return nil
}
