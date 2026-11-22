package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// aiPageNames is the entities service as the AI tools see it. The importer
// and record kinds name a page and ask GetBySlug for Slugify(name), but a page
// whose slug took a uniqueness suffix (an older page of the same name in the
// trash keeps the plain slug) has another slug, while the lookups, which match
// by name, still show it. GetBySlug here resolves the name the same way.
type aiPageNames struct{ entities.EntityService }

// GetBySlug is the live page whose name gives slug.
func (p aiPageNames) GetBySlug(ctx context.Context, campaignID, slug string) (*entities.Entity, error) {
	return p.GetByNameSlug(ctx, campaignID, slug)
}
