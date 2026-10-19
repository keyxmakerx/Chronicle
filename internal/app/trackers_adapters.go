package app

// trackers_adapters.go bridges the trackers plugin to the entities and
// calendar services. The plugin owns the narrow interfaces; the adapters live
// here so it never imports another plugin.

import (
	"context"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

// trackersEntityAdapter implements trackers.EntityDirectory.
type trackersEntityAdapter struct {
	svc   entities.EntityService
	cards entities.PageCards
}

// Access applies the page's own visibility rule. A page of another campaign
// is reported as absent, so a foreign id never resolves through this plugin.
func (a *trackersEntityAdapter) Access(ctx context.Context, campaignID, entityID string, v trackers.Viewer) (trackers.PageAccess, error) {
	cards, err := a.cards.InCampaign(ctx, campaignID, []string{entityID})
	if err != nil {
		return trackers.PageAccess{}, err
	}
	if _, ok := cards[entityID]; !ok {
		return trackers.PageAccess{}, nil
	}
	ep, err := a.svc.CheckEntityAccess(ctx, entityID, v.VisibilityRole, v.UserID)
	if err != nil {
		return trackers.PageAccess{}, err
	}
	if !ep.CanView {
		return trackers.PageAccess{Exists: true}, nil
	}
	e, err := a.svc.GetByID(ctx, entityID)
	if err != nil {
		return trackers.PageAccess{}, err
	}
	return trackers.PageAccess{Exists: true, CanView: true, TypeID: e.EntityTypeID}, nil
}

// Names names the campaign's pages among ids.
func (a *trackersEntityAdapter) Names(ctx context.Context, campaignID string, ids []string) (map[string]string, error) {
	cards, err := a.cards.InCampaign(ctx, campaignID, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(cards))
	for id, c := range cards {
		out[id] = c.Name
	}
	return out, nil
}

// Types lists the campaign's page categories.
func (a *trackersEntityAdapter) Types(ctx context.Context, campaignID string) ([]trackers.TypeRef, error) {
	list, err := a.svc.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]trackers.TypeRef, 0, len(list))
	for _, t := range list {
		out = append(out, trackers.TypeRef{ID: t.ID, Name: t.Name})
	}
	return out, nil
}

// trackersClockAdapter implements trackers.GameClock over the campaign's
// default calendar, as the writer may see it.
type trackersClockAdapter struct {
	svc calendar.CalendarService
}

func (a *trackersClockAdapter) Today(ctx context.Context, campaignID string, v trackers.Viewer) (*trackers.GameDate, error) {
	cal, err := a.svc.GetDefaultCalendarForViewer(ctx, campaignID, permissions.RequestViewer(v.VisibilityRole, v.UserID))
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &trackers.GameDate{
		Year: cal.CurrentYear, Month: cal.CurrentMonth, Day: cal.CurrentDay,
		Label: cal.FullDateLabel(),
	}, nil
}

// registerTrackersBlock adds the Trackers block to page layouts. The widget
// fetches the page's trackers itself, so the block only emits its mount.
func registerTrackersBlock(blockRegistry *entities.BlockRegistry) {
	blockRegistry.Register(entities.BlockMeta{
		Type: trackers.BlockType, Label: "Trackers", Icon: "fa-gauge",
		Description: "Counters, named steps and clocks on this page, each with a dated log",
		Contexts:    []string{"template"},
		Singleton:   true,
		TileSizes:   []string{"1x1", "2x1", "2x2", "4x1"},
	}, func(rc entities.BlockRenderContext) templ.Component {
		if rc.CC == nil || rc.CC.Campaign == nil || rc.Entity == nil {
			return templ.NopComponent
		}
		return trackers.BlockMount(rc.CC.Campaign.ID, rc.Entity.ID, rc.CSRFToken)
	})
}
