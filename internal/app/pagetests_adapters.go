package app

// pagetests_adapters.go bridges the Tests block to the entities and calendar
// services, and lends its recorded results to the encounters plugin. The
// plugins own the narrow interfaces; the adapters live here so neither
// imports another plugin.

import (
	"context"
	"log/slog"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/encounters"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/pagetests"
)

// pageTestsPageAdapter implements pagetests.PageDirectory with the same rules
// the encounters block uses for its page.
type pageTestsPageAdapter struct{ pages *encountersPageAdapter }

func encounterViewerOf(v pagetests.Viewer) encounters.Viewer {
	return encounters.Viewer{UserID: v.UserID, VisibilityRole: v.VisibilityRole}
}

func (a *pageTestsPageAdapter) Access(ctx context.Context, campaignID, entityID string, v pagetests.Viewer) (pagetests.PageAccess, error) {
	got, err := a.pages.Access(ctx, campaignID, entityID, encounterViewerOf(v))
	return pagetests.PageAccess{Exists: got.Exists, CanView: got.CanView, CanEdit: got.CanEdit}, err
}

func (a *pageTestsPageAdapter) Names(ctx context.Context, campaignID string, ids []string, v pagetests.Viewer) (map[string]string, error) {
	return a.pages.Names(ctx, campaignID, ids, encounterViewerOf(v))
}

// pageTestsCalendar implements pagetests.Calendar over the primary calendar,
// read the way quest due dates read it: none when the addon is off or the
// campaign has no calendar.
type pageTestsCalendar struct{ cal *questCalendarAdapter }

func (a pageTestsCalendar) Today(ctx context.Context, campaignID string) (pagetests.Day, string, bool, error) {
	qc, err := a.cal.Calendar(ctx, campaignID)
	if err != nil || qc == nil {
		return pagetests.Day{}, "", false, err
	}
	d := qc.Today()
	return pagetests.Day{Year: d.Year, Month: d.Month, Day: d.Day}, qc.Label(d), true, nil
}

// encounterResultsAdapter implements encounters.ResultDirectory over the
// tests service, so an encounter can name the test and result that pick
// each of its versions. With the Tests block degraded it is nil and the
// encounters plugin reads every pick as unknown.
type encounterResultsAdapter struct{ svc pagetests.Service }

func (a encounterResultsAdapter) Outcomes(ctx context.Context, campaignID string, ids []string, v encounters.Viewer) (map[string]encounters.TestOutcome, error) {
	got, err := a.svc.Outcomes(ctx, campaignID, ids, pagetests.Viewer{UserID: v.UserID, VisibilityRole: v.VisibilityRole})
	if err != nil {
		return nil, err
	}
	out := make(map[string]encounters.TestOutcome, len(got))
	for id, o := range got {
		t := encounters.TestOutcome{Name: o.Name, Recorded: o.Recorded, Results: map[string]string{}}
		for _, r := range o.Results {
			t.Results[r.ID] = r.Label
		}
		out[id] = t
	}
	return out, nil
}

// registerPageTestsBlock registers the Tests block: the long tests made on a
// page and the results they ended in. It is one per page, because its list
// is a single sheet. What a viewer is told is decided by the tests service;
// the block draws nothing for a viewer who has nothing to see and may not
// add a test, so a player's page has no empty box.
func registerPageTestsBlock(blockRegistry *entities.BlockRegistry, svc pagetests.Service) {
	blockRegistry.Register(entities.BlockMeta{
		Type: pagetests.BlockType, Label: "Tests", Icon: "fa-flag-checkered",
		Description: "Montages and other long tests on this page, and how they ended",
		Contexts:    []string{"template"},
		TileSizes:   []string{"2x2", "4x2", "1x2"},
		Singleton:   true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		if rc.CC == nil || rc.CC.Campaign == nil || rc.Entity == nil {
			return templ.NopComponent
		}
		list, err := svc.ListForPage(context.Background(), rc.CC.Campaign.ID, rc.Entity.ID, pagetests.ViewerOf(rc.CC, rc.UserID))
		if err != nil {
			slog.Error("page tests: cannot read the page's tests",
				slog.String("entity_id", rc.Entity.ID), slog.Any("error", err))
			return templ.NopComponent
		}
		if len(list.Tests) == 0 && !list.CanEdit {
			return templ.NopComponent
		}
		return pagetests.BlockMount(rc.CC.Campaign.ID, rc.Entity.ID, rc.CSRFToken)
	})
}
