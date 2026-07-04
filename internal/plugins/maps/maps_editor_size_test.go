package maps

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestMapEditorBody_InvalidatesSizeAfterLayout pins the maps #9 fix. The
// editor's inline Leaflet IIFE runs at parse time, before the flex-sized
// container (dedicated page) or a freshly-shown embed has its final height,
// so Leaflet reads the wrong size at construction: the image overlay paints
// into the wrong box (the container's bg-surface-alt shows through as a pale
// rectangle) and markers land at the wrong pixels (piling near the toolbar).
// The editor must re-measure once layout settles — the same guard the
// dashboard map widget (map_widget.js) already uses.
func TestMapEditorBody_InvalidatesSizeAfterLayout(t *testing.T) {
	cc := &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RoleScribe,
	}
	data := MapViewData{
		CampaignID: "camp-1",
		Map:        &Map{ID: "m-1", CampaignID: "camp-1", Name: "Overworld"},
		IsScribe:   true,
	}

	out := render(t, MapEditorBody(cc, data, "flex-1 relative bg-surface-alt", ""))

	if !strings.Contains(out, "invalidateSize") {
		t.Errorf("editor IIFE must call map.invalidateSize() after layout settles; not found in rendered output")
	}
}
