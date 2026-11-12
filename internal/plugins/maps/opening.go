package maps

import (
	"context"
	"fmt"

	"github.com/a-h/templ"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// mapsOpening is the Maps page's opening: its name, the three sentences it
// shows while empty, and the one action, offered only to the owner, who is
// the only one who can create a map.
func mapsOpening(ctx context.Context, data MapListData) layouts.AppOpeningData {
	gm := layouts.GMTerm(ctx)
	d := layouts.AppOpeningData{
		Name:  "Maps",
		Empty: len(data.Maps) == 0,
		Intros: layouts.AppIntros{
			Player: fmt.Sprintf("Maps your %s shares will show here, with the places you've been pinned on them.", gm),
			CoDM:   "Maps the owner draws or uploads will show here, with pages pinned on them.",
			Owner:  "Draw or upload a map, pin pages to it, and choose what players can see.",
		},
	}
	if data.IsOwner {
		d.Action = &layouts.AppAction{
			Label: "Add a map",
			Icon:  "fa-plus",
			Attrs: templ.Attributes{"onclick": "document.getElementById('create-map-modal').classList.remove('hidden')"},
		}
	}
	return d
}

// mapCount is the count beside the title when there are maps.
func mapCount(n int) string {
	if n == 1 {
		return "1 map"
	}
	return fmt.Sprintf("%d maps", n)
}
