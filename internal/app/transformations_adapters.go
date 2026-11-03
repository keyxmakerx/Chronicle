package app

// transformations_adapters.go bridges hero layers to the entities service.
// The plugin owns the narrow interface; the adapter lives here so neither
// imports the other.

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/encounters"
	"github.com/keyxmakerx/chronicle/internal/plugins/transformations"
)

// transformationsPageAdapter implements transformations.PageDirectory with
// the same page rules the encounters and Tests blocks use.
type transformationsPageAdapter struct{ pages *encountersPageAdapter }

func (a *transformationsPageAdapter) Access(ctx context.Context, campaignID, entityID string, v transformations.Viewer) (transformations.PageAccess, error) {
	got, err := a.pages.Access(ctx, campaignID, entityID, encounters.Viewer{UserID: v.UserID, VisibilityRole: v.VisibilityRole})
	return transformations.PageAccess{Exists: got.Exists, CanView: got.CanView, CanEdit: got.CanEdit}, err
}
