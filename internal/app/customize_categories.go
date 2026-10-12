package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// customizeSectionCategories is the draft key for the campaign's categories.
const customizeSectionCategories = "categories"

// customizeCategoryAdapter lets the campaigns plugin list categories without
// importing entities (T-B2).
type customizeCategoryAdapter struct {
	svc entities.EntityService
}

// ListCustomizeCategories returns the campaign's categories for the Customize page.
func (a *customizeCategoryAdapter) ListCustomizeCategories(ctx context.Context, campaignID string) ([]campaigns.CustomizeCategory, error) {
	infos, err := a.svc.CustomizeCategories(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]campaigns.CustomizeCategory, len(infos))
	for i, in := range infos {
		out[i] = campaigns.CustomizeCategory{
			ID: in.Type.ID, Slug: in.Type.Slug, Name: in.Type.Name, NamePlural: in.Type.NamePlural,
			Icon: in.Type.Icon, Color: in.Type.Color, ParentID: in.Type.ParentTypeID,
			Claimable: in.Claimable, Pages: in.Pages, Trashed: in.Trashed, IsDefault: in.Type.IsDefault,
			DMOnly: in.Type.DMOnly(),
		}
	}
	return out, nil
}

// categoriesApplier is the slice of the entity service the section needs.
type categoriesApplier interface {
	ApplyCustomizeCategories(ctx context.Context, campaignID string, in entities.CustomizeCategoriesPatch) (entities.CustomizeCategoriesResult, error)
}

// categoryMenu is the slice of the campaign service that owns the sidebar.
type categoryMenu interface {
	SetCategoryMenuVisibility(ctx context.Context, campaignID string, typeID int, visible bool) error
}

// categoriesSectionValue is the section as the page stores it. Changes is the
// page's own list of labels for its save bar, carried through untouched.
type categoriesSectionValue struct {
	Items   map[string]entities.CustomizeCategoryItem `json:"items"`
	Changes json.RawMessage                           `json:"_changes,omitempty"`
}

// newCategoriesSection builds the "categories" Customize section. The entity
// service owns the categories and the campaign service owns the sidebar menu;
// this is the one place that joins the two for a Save.
func newCategoriesSection(ent categoriesApplier, menu categoryMenu) campaigns.CustomizeSection {
	return campaigns.CustomizeSection{
		Key: customizeSectionCategories,
		Apply: func(ctx context.Context, cc *campaigns.CampaignContext, _ string, raw json.RawMessage) (json.RawMessage, error) {
			if cc.MemberRole < campaigns.RoleOwner {
				return nil, apperror.NewForbidden("only campaign owners can change categories")
			}
			return applyCategoriesSection(ctx, cc.Campaign.ID, ent, menu, raw)
		},
	}
}

// applyCategoriesSection applies the section and returns what is still unapplied.
func applyCategoriesSection(ctx context.Context, campaignID string, ent categoriesApplier, menu categoryMenu, raw json.RawMessage) (json.RawMessage, error) {
	var sec categoriesSectionValue
	if err := json.Unmarshal(raw, &sec); err != nil {
		return nil, apperror.NewBadRequest("the categories in the draft are not valid")
	}
	if len(sec.Items) == 0 {
		return nil, nil
	}

	res, applyErr := ent.ApplyCustomizeCategories(ctx, campaignID, entities.CustomizeCategoriesPatch{Items: sec.Items})

	applied := make(map[string]bool, len(res.Applied))
	for _, k := range res.Applied {
		applied[k] = true
	}

	// The menu setting follows the category it belongs to, so it runs only
	// for items that applied. A failure keeps just the menu part in the draft.
	menuLeft := map[string]entities.CustomizeCategoryItem{}
	var menuErr error
	for _, key := range res.Applied {
		it := sec.Items[key]
		if it.Menu == nil || it.Delete {
			continue
		}
		id, ok := res.Created[key]
		if !ok {
			id, _ = strconv.Atoi(key)
		}
		if err := menu.SetCategoryMenuVisibility(ctx, campaignID, id, *it.Menu); err != nil {
			slog.Warn("customize: category menu setting failed", slog.String("campaign_id", campaignID), slog.Int("type_id", id), slog.Any("error", err))
			menuLeft[key] = entities.CustomizeCategoryItem{Menu: it.Menu}
			if menuErr == nil {
				menuErr = err
			}
		}
	}

	remaining := map[string]entities.CustomizeCategoryItem{}
	for key, it := range sec.Items {
		newKey := key
		if id, created := res.Created[key]; created {
			// Made in the database now, so the draft must name it by id; the
			// children still waiting point at that id too.
			newKey = strconv.Itoa(id)
			if applied[key] {
				if left, ok := menuLeft[key]; ok {
					remaining[newKey] = left
				}
				continue
			}
			it.New, it.CopyFrom = false, 0
		} else if applied[key] {
			if left, ok := menuLeft[key]; ok {
				remaining[newKey] = left
			}
			continue
		}
		if id, ok := res.Created[it.Parent]; ok {
			it.Parent = strconv.Itoa(id)
		}
		remaining[newKey] = it
	}

	err := applyErr
	if err == nil {
		err = menuErr
	}
	if len(remaining) == 0 {
		return nil, err
	}
	if err != nil && len(res.Applied) == 0 && len(res.Created) == 0 {
		return nil, err
	}
	out, mErr := json.Marshal(categoriesSectionValue{Items: remaining, Changes: sec.Changes})
	if mErr != nil {
		return nil, apperror.NewInternal(fmt.Errorf("marshaling the remaining categories: %w", mErr))
	}
	return out, err
}
