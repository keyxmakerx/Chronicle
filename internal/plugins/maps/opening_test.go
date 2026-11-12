package maps

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// A viewer who can't create maps is never shown a control to create one, and
// the empty page tells them who adds maps, in the game master's own word.
func TestMapListOpening_ByViewer(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", Name: "The Ashen Reach"}}
	base := layouts.SetGMTerm(context.Background(), "Director")
	tests := []struct {
		name     string
		ctx      context.Context
		owner    bool
		want     []string
		notWant  []string
		wantCtrl bool
	}{
		{"player", base, false, []string{"Maps your Director shares will show here", "Nothing to do yet"}, []string{"Add a map", "New Map", "Create", "create-map-modal"}, false},
		{"co-DM", layouts.SetIsDmGranted(base, true), false, []string{"Maps the owner draws or uploads"}, []string{"Add a map", "New Map", "create-map-modal"}, false},
		{"owner", layouts.SetIsOwner(base, true), true, []string{"Draw or upload a map", "Add a map", "create-map-modal"}, []string{"Nothing to do yet"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			data := MapListData{CampaignID: "c1", IsOwner: tt.owner}
			if err := mapListContent(cc, data).Render(tt.ctx, &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(out, w) {
					t.Errorf("a viewer who can't create maps sees %q", w)
				}
			}
		})
	}
}

// With maps the page is one title line with the count beside it.
func TestMapListOpening_WithMaps(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}}
	var buf bytes.Buffer
	data := MapListData{CampaignID: "c1", Maps: []Map{{ID: "m1", Name: "North"}, {ID: "m2", Name: "South"}}}
	if err := mapListContent(cc, data).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `data-app-opening="full"`) || !strings.Contains(out, "2 maps") || strings.Contains(out, "No maps yet") {
		t.Errorf("unexpected opening:\n%s", out)
	}
}
