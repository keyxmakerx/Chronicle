package app

// encounters_adapters.go bridges the encounters plugin to the entities
// service and the sync API. The plugins own the narrow interfaces; the
// adapters live here so neither imports another plugin.

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/encounters"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

// encountersPageAdapter implements encounters.PageDirectory.
type encountersPageAdapter struct {
	svc   entities.EntityService
	cards entities.PageCards
}

// Access applies the rules the page itself uses: a page of another campaign
// is absent and CheckEntityAccess decides viewing and editing. The page
// blocks also require Scribe or above to edit; the encounters service
// already requires Owner visibility, which is higher.
func (a *encountersPageAdapter) Access(ctx context.Context, campaignID, entityID string, v encounters.Viewer) (encounters.PageAccess, error) {
	cards, err := a.cards.InCampaign(ctx, campaignID, []string{entityID})
	if err != nil {
		return encounters.PageAccess{}, err
	}
	if _, ok := cards[entityID]; !ok {
		return encounters.PageAccess{}, nil
	}
	ep, err := a.svc.CheckEntityAccess(ctx, entityID, v.VisibilityRole, v.UserID)
	if err != nil {
		return encounters.PageAccess{}, err
	}
	return encounters.PageAccess{
		Exists:  true,
		CanView: ep.CanView,
		CanEdit: ep.CanView && ep.CanEdit,
	}, nil
}

// Names names the live pages of the campaign among ids that the viewer may
// see, in one query for the names and one for visibility.
func (a *encountersPageAdapter) Names(ctx context.Context, campaignID string, ids []string, v encounters.Viewer) (map[string]string, error) {
	cards, err := a.cards.InCampaign(ctx, campaignID, ids)
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return map[string]string{}, nil
	}
	live := make([]string, 0, len(cards))
	for id := range cards {
		live = append(live, id)
	}
	ok, err := a.svc.FilterViewableEntityIDs(ctx, campaignID, live, v.VisibilityRole, v.UserID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(ok))
	for id, card := range cards {
		if ok[id] {
			out[id] = card.Name
		}
	}
	return out, nil
}

// syncEncounterAPIAdapter implements syncapi.EncounterAPIService. The sync
// API handler has already checked that the caller is the owner or a co-DM,
// so the caller reads as the DM. The routes are registered before the
// encounters plugin is built, so the service is attached afterwards; until
// then (or when the plugin is degraded) every call reads as not found.
type syncEncounterAPIAdapter struct {
	svc encounters.Service
}

var _ syncapi.EncounterAPIService = (*syncEncounterAPIAdapter)(nil)

func (a *syncEncounterAPIAdapter) attach(svc encounters.Service) { a.svc = svc }

func encounterAPIViewer(userID string) encounters.Viewer {
	return encounters.Viewer{UserID: userID, VisibilityRole: permissions.RoleOwner}
}

func (a *syncEncounterAPIAdapter) Search(ctx context.Context, campaignID, userID, q string, limit int) (any, int, error) {
	if a.svc == nil {
		return nil, 0, apperror.NewNotFound("encounters are not available")
	}
	return a.svc.Search(ctx, campaignID, q, limit, encounterAPIViewer(userID))
}

func (a *syncEncounterAPIAdapter) Get(ctx context.Context, campaignID, userID, id string) (any, error) {
	if a.svc == nil {
		return nil, apperror.NewNotFound("encounters are not available")
	}
	return a.svc.Get(ctx, campaignID, id, encounterAPIViewer(userID))
}
