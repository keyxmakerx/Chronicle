package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// aiCategoryAdapter implements records.CategoryAPI over the entities
// service, with the owner check the sidebar menu's adapter makes.
type aiCategoryAdapter struct {
	owners interface {
		IsOwner(ctx context.Context, campaignID, userID string) (bool, error)
	}
	types interface {
		CustomizeCategories(ctx context.Context, campaignID string) ([]entities.CustomizeCategoryInfo, error)
		DeleteEntityType(ctx context.Context, id int) error
		MergeEntityType(ctx context.Context, campaignID string, fromID, toID int) (int, error)
	}
}

var _ records.CategoryAPI = aiCategoryAdapter{}

func (a aiCategoryAdapter) IsOwner(ctx context.Context, campaignID, userID string) (bool, error) {
	return a.owners.IsOwner(ctx, campaignID, userID)
}

// Categories reads the Customizer's list, which counts every page whatever
// its visibility and the Trash apart, so the review row counts what moves.
func (a aiCategoryAdapter) Categories(ctx context.Context, campaignID string) ([]records.CategoryInfo, error) {
	infos, err := a.types.CustomizeCategories(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return categoryInfos(infos), nil
}

func categoryInfos(infos []entities.CustomizeCategoryInfo) []records.CategoryInfo {
	names := make(map[int]string, len(infos))
	for _, in := range infos {
		names[in.Type.ID] = categoryMenuName(in.Type)
	}
	kinds := map[int][]string{}
	for _, in := range infos {
		if p := in.Type.ParentTypeID; p != nil {
			kinds[*p] = append(kinds[*p], categoryMenuName(in.Type))
		}
	}
	out := make([]records.CategoryInfo, 0, len(infos))
	for _, in := range infos {
		t := in.Type
		c := records.CategoryInfo{
			ID: t.ID, Name: t.Name, Plural: t.NamePlural, Slug: t.Slug,
			Kinds: kinds[t.ID], DMOnly: t.DMOnly(), Pages: in.Pages, Trashed: in.Trashed,
		}
		if t.ParentTypeID != nil {
			c.Parent = names[*t.ParentTypeID]
		}
		out = append(out, c)
	}
	return out
}

func categoryMenuName(t entities.EntityType) string {
	if t.NamePlural != "" {
		return t.NamePlural
	}
	return t.Name
}

// DeleteCategory checks the category is this campaign's first: the service's
// delete takes an id alone.
func (a aiCategoryAdapter) DeleteCategory(ctx context.Context, campaignID string, id int) error {
	infos, err := a.types.CustomizeCategories(ctx, campaignID)
	if err != nil {
		return err
	}
	for _, in := range infos {
		if in.Type.ID == id {
			return a.types.DeleteEntityType(ctx, id)
		}
	}
	return apperror.NewNotFound("category not found")
}

func (a aiCategoryAdapter) MergeCategory(ctx context.Context, campaignID string, fromID, toID int) (int, error) {
	return a.types.MergeEntityType(ctx, campaignID, fromID, toID)
}
