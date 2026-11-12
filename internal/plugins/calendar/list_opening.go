package calendar

import (
	"context"
	"fmt"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// calendarsOpening is the Calendars page's opening. The list keeps its own
// look below; only the opening line is shared. For a non-owner an empty list
// may mean every calendar is hidden from them, so the player sentence says
// what shows here and never that none exist.
func calendarsOpening(ctx context.Context, data CalendarsListData) layouts.AppOpeningData {
	d := layouts.AppOpeningData{
		Name:  "Calendars",
		Empty: len(data.Calendars) == 0,
		Intros: layouts.AppIntros{
			Player: fmt.Sprintf("When your %s shares a calendar with you, it shows here.", layouts.GMTerm(ctx)),
			CoDM:   "Calendars the owner shares with you show here.",
			Owner:  "Start from a preset, or import one from Simple Calendar, Calendaria or a Chronicle export.",
		},
	}
	if data.IsOwner {
		d.Action = &layouts.AppAction{
			Label: "New calendar",
			Icon:  "fa-plus",
			Attrs: templ.Attributes{
				"hx-get":    fmt.Sprintf("/campaigns/%s/calendars/wizard", data.CampaignID),
				"hx-target": "#calv5-wizard-root",
				"hx-swap":   "outerHTML",
				"onclick":   "window.__calv5Opener=this",
			},
		}
	}
	return d
}
