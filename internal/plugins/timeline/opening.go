package timeline

import (
	"context"
	"fmt"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// timelinesOpening is the Timelines page's opening: its name, the three
// sentences it shows while empty, and the one action, offered only to the
// owner, who is the only one who can create a timeline.
func timelinesOpening(ctx context.Context, data TimelineListData) layouts.AppOpeningData {
	gm := layouts.GMTerm(ctx)
	d := layouts.AppOpeningData{
		Name:  "Timelines",
		Empty: len(data.Timelines) == 0,
		Intros: layouts.AppIntros{
			Player: fmt.Sprintf("Timelines your %s shares will show here, with the events your table has lived through.", gm),
			CoDM:   "Timelines the owner adds will show here, with the events they hold.",
			Owner:  "Lay out events along a line, link them to your calendar, and choose what players can see.",
		},
	}
	if data.IsOwner {
		d.Action = &layouts.AppAction{
			Label: "Add a timeline",
			Icon:  "fa-plus",
			Attrs: templ.Attributes{"onclick": "document.getElementById('create-timeline-modal').classList.remove('hidden')"},
		}
	}
	return d
}

// timelineCount is the count beside the title when there are timelines.
func timelineCount(n int) string {
	if n == 1 {
		return "1 timeline"
	}
	return fmt.Sprintf("%d timelines", n)
}
