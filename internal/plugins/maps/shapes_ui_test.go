package maps

import (
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The Shapes palette is part of the drawing tools, so it is on the rail only
// for people who may draw, and the viewer gets the module's address. The test
// map has no picture: a blank map must render the same page with the same
// tools (it is drawn from nothing, so there is nothing to wait for).
func TestShow_ShapesPaletteFollowsTheDrawGate(t *testing.T) {
	cases := []struct {
		name string
		cc   *campaigns.CampaignContext
		want bool
	}{
		{"player", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer}, false},
		{"scribe", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe}, true},
		{"owner", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandler(newTestMapService(viewerRepo()))
			rec, err := serveHandler(t, h.Show, "/campaigns/camp-1/maps/m-1", tc.cc, "m-1")
			if err != nil || rec.Code != http.StatusOK {
				t.Fatalf("Show: code=%d err=%v", rec.Code, err)
			}
			page := rec.Body.String()
			if !strings.Contains(page, `data-image-url=""`) {
				t.Fatal("the fixture map must be blank (no picture)")
			}
			if !strings.Contains(page, `data-shapes-src=`) {
				t.Error("the viewer is not told where the shapes module is")
			}
			for _, kind := range []string{"room", "area", "water", "wall", "arrow", "bubble"} {
				has := strings.Contains(page, `data-pshape="`+kind+`"`)
				if has != tc.want {
					t.Errorf("palette button %q present = %v, want %v", kind, has, tc.want)
				}
			}
			if got := strings.Contains(page, `data-tool="shapes"`); got != tc.want {
				t.Errorf("Shapes rail button present = %v, want %v", got, tc.want)
			}
		})
	}
}
