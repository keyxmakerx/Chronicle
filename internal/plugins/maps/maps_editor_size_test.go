package maps

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestMapEditorBody_InvalidatesSizeAfterLayout pins that the editor's inline
// Leaflet IIFE re-measures once layout settles. It runs at parse time, before
// a flex-sized container or freshly-shown embed has its final height, so
// without a re-measure Leaflet reads the wrong size at construction (image
// overlay in the wrong box, markers at the wrong pixels) — the same guard the
// dashboard map widget (map_widget.js) uses.
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

	out := render(t, MapEditorBody(cc, data, "flex-1 relative bg-surface-alt", "")) + viewerScript(t)

	if !strings.Contains(out, "invalidateSize") {
		t.Errorf("editor IIFE must call map.invalidateSize() after layout settles; not found in rendered output")
	}
}
