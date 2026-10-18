package app

// encounters_adapters.go bridges the encounters plugin to the entities
// service and the sync API. The plugins own the narrow interfaces; the
// adapters live here so neither imports another plugin.

import (
	"context"
	"strconv"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
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

// aiEncountersAdapter implements records.EncountersAPI over the encounters
// service, acting as the operator; the service applies its own DM rule.
type aiEncountersAdapter struct{ svc encounters.Service }

var _ records.EncountersAPI = aiEncountersAdapter{}

func aiEncounterViewer(a records.Actor) encounters.Viewer {
	return encounters.Viewer{UserID: a.UserID, VisibilityRole: a.Role}
}

func aiEncounterRef(e encounters.Encounter) records.EncounterRef {
	ref := records.EncounterRef{ID: e.ID, Name: e.Name, PageName: e.Page.Name, Ready: e.Ready, Version: e.Version}
	for _, h := range e.Heroes {
		ref.Heroes = append(ref.Heroes, h.Name)
		ref.HeroIDs = append(ref.HeroIDs, h.ID)
	}
	for _, c := range e.Creatures {
		ref.Creatures = append(ref.Creatures, records.EncounterCreature{
			EntityID: c.EntityID, Name: c.Name, Count: c.Count, Group: c.Group, Start: c.Start,
		})
	}
	return ref
}

// aiEncounterSave builds the whole-encounter body; rows get fresh ids,
// since an AI block replaces the list rather than editing rows.
func aiEncounterSave(in records.EncounterInput) encounters.SaveInput {
	out := encounters.SaveInput{Name: in.Name, Ready: in.Ready, Heroes: in.HeroIDs, Creatures: []encounters.Creature{}}
	for i, c := range in.Creatures {
		out.Creatures = append(out.Creatures, encounters.Creature{
			ID: "ai" + strconv.Itoa(i+1), EntityID: c.EntityID, Name: c.Name, Count: c.Count, Group: c.Group, Start: c.Start,
		})
	}
	return out
}

// keepRowIDs gives each new row the id of an old row for the same page, in
// order, so stages that name the old row still find it. Other rows get an
// id no old row had.
func keepRowIDs(rows, old []encounters.Creature) {
	taken := map[string]bool{}
	for _, o := range old {
		taken[o.ID] = true
	}
	kept := map[string]bool{}
	matched := make([]bool, len(rows))
	for i := range rows {
		for _, o := range old {
			if !kept[o.ID] && o.EntityID != "" && o.EntityID == rows[i].EntityID {
				kept[o.ID], matched[i] = true, true
				rows[i].ID = o.ID
				break
			}
		}
	}
	n := 0
	for i := range rows {
		if matched[i] {
			continue
		}
		for {
			n++
			if id := "ai" + strconv.Itoa(n); !taken[id] {
				rows[i].ID, taken[id] = id, true
				break
			}
		}
	}
}

func (a aiEncountersAdapter) ListForPage(ctx context.Context, campaignID, pageID string, act records.Actor) ([]records.EncounterRef, error) {
	list, err := a.svc.ListForPage(ctx, campaignID, pageID, aiEncounterViewer(act))
	if err != nil {
		return nil, err
	}
	out := make([]records.EncounterRef, 0, len(list.Encounters))
	for _, e := range list.Encounters {
		out = append(out, aiEncounterRef(e))
	}
	return out, nil
}

func (a aiEncountersAdapter) Create(ctx context.Context, campaignID, pageID string, act records.Actor, in records.EncounterInput) error {
	_, err := a.svc.Create(ctx, campaignID, pageID, aiEncounterViewer(act), aiEncounterSave(in))
	return err
}

func (a aiEncountersAdapter) Update(ctx context.Context, campaignID, id string, act records.Actor, in records.EncounterInput, version int) error {
	body := aiEncounterSave(in)
	body.Version = &version
	// An AI block knows nothing of stages, so the stages the DM planned are
	// kept: a row for the same page keeps its old id, and what the stages
	// still point at survives.
	cur, err := a.svc.Get(ctx, campaignID, id, aiEncounterViewer(act))
	if err != nil {
		return err
	}
	keepRowIDs(body.Creatures, cur.Creatures)
	body.Stages = encounters.CarryStages(cur.Stages, body.Creatures)
	_, err = a.svc.Update(ctx, campaignID, id, aiEncounterViewer(act), body)
	return err
}

func (a aiEncountersAdapter) Delete(ctx context.Context, campaignID, id string, act records.Actor) error {
	return a.svc.Delete(ctx, campaignID, id, aiEncounterViewer(act))
}

// All reads the newest encounters, up to the search cap, one by one; the
// export is rare and an AI only needs the names to update by.
func (a aiEncountersAdapter) All(ctx context.Context, campaignID string, act records.Actor) ([]records.EncounterRef, error) {
	v := aiEncounterViewer(act)
	items, _, err := a.svc.Search(ctx, campaignID, "", encounters.MaxSearchLimit, v)
	if err != nil {
		return nil, err
	}
	out := make([]records.EncounterRef, 0, len(items))
	for _, it := range items {
		e, err := a.svc.Get(ctx, campaignID, it.ID, v)
		if err != nil {
			return nil, err
		}
		out = append(out, aiEncounterRef(*e))
	}
	return out, nil
}
