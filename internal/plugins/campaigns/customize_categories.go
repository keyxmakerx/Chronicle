package campaigns

import (
	"context"
	"log/slog"
)

// CustomizeCategory is one category as the Customize page lists it: the
// fields the owner can edit plus the counts that decide whether it can be
// deleted. Claimable is the effective value, after the heuristics that apply
// when the owner never chose.
type CustomizeCategory struct {
	ID         int    `json:"id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	NamePlural string `json:"namePlural"`
	Icon       string `json:"icon"`
	Color      string `json:"color"`
	ParentID   *int   `json:"parentId,omitempty"`
	Claimable  bool   `json:"claimable"`
	// Pages counts live pages and Trashed those waiting in the Trash; a
	// category with either cannot be deleted.
	Pages     int  `json:"pages"`
	Trashed   int  `json:"trashed"`
	IsDefault bool `json:"isDefault"`
}

// CustomizeCategoryLister lists a campaign's categories for the Customize
// page. It is an interface so the campaigns plugin never imports entities.
type CustomizeCategoryLister interface {
	ListCustomizeCategories(ctx context.Context, campaignID string) ([]CustomizeCategory, error)
}

// SetCustomizeCategoryLister wires the category source for the Customize page.
func (h *Handler) SetCustomizeCategoryLister(l CustomizeCategoryLister) {
	h.categoryLister = l
}

// CustomizeCategories lists the campaign's categories for the Customize page.
// A failure is logged and reads as none, so the page still opens.
func (h *Handler) CustomizeCategories(ctx context.Context, campaignID string) []CustomizeCategory {
	if h.categoryLister == nil {
		return nil
	}
	cats, err := h.categoryLister.ListCustomizeCategories(ctx, campaignID)
	if err != nil {
		slog.Error("listing customize categories failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return nil
	}
	return cats
}

// CategoryMenuVisible reports, per category id, the menu setting of every
// category that has a menu item. A category with no item is absent from the
// map and counts as visible, exactly as NormalizeNav treats it, so read it as
// "visible unless the map says false". When an id has several items the first
// wins, as in NormalizeNav.
func (h *Handler) CategoryMenuVisible(cc *CampaignContext) map[int]bool {
	out := map[int]bool{}
	for _, it := range cc.Campaign.ParseSidebarConfig().Items {
		if it.Type != SidebarTypeCategory || it.TypeID <= 0 {
			continue
		}
		if _, seen := out[it.TypeID]; !seen {
			out[it.TypeID] = it.Visible
		}
	}
	return out
}
