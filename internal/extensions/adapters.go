// Package extensions — adapters.go bridges external service interfaces to
// the applier's narrower interfaces. This avoids importing heavy plugin
// packages directly.
package extensions

import "context"

// entityTypeAdapter wraps any service with a CreateEntityType method
// matching the entities.EntityService interface.
type entityTypeAdapter struct {
	create func(ctx context.Context, campaignID string, name, namePlural, icon, color string) (id int, slug string, err error)
}

// NewEntityTypeAdapter creates an adapter from a creation function.
// The caller extracts the function from the concrete entity service.
func NewEntityTypeAdapter(
	create func(ctx context.Context, campaignID string, name, namePlural, icon, color string) (int, string, error),
) EntityTypeCreator {
	return &entityTypeAdapter{create: create}
}

// CreateEntityType implements EntityTypeCreator.
func (a *entityTypeAdapter) CreateEntityType(ctx context.Context, campaignID string, input EntityTypeCreateInput) (EntityTypeResult, error) {
	id, slug, err := a.create(ctx, campaignID, input.Name, input.NamePlural, input.Icon, input.Color)
	if err != nil {
		return EntityTypeResult{}, err
	}
	return EntityTypeResult{ID: id, Slug: slug}, nil
}

// tagAdapter wraps any service with a Create method matching the
// tags.TagService interface.
type tagAdapter struct {
	create func(ctx context.Context, campaignID string, name, color string, dmOnly bool) (int, error)
}

// NewTagAdapter creates an adapter from a creation function.
func NewTagAdapter(
	create func(ctx context.Context, campaignID string, name, color string, dmOnly bool) (int, error),
) TagCreator {
	return &tagAdapter{create: create}
}

// CreateTag implements TagCreator.
func (a *tagAdapter) CreateTag(ctx context.Context, campaignID string, name, color string, dmOnly bool) (TagResult, error) {
	id, err := a.create(ctx, campaignID, name, color, dmOnly)
	if err != nil {
		return TagResult{}, err
	}
	return TagResult{ID: id}, nil
}
