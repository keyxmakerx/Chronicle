package timeline

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// A viewer who can't create timelines is never shown a control to create one,
// and the empty page tells them who adds timelines, in the game master's word.
func TestTimelineListOpening_ByViewer(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", Name: "The Ashen Reach"}}
	base := layouts.SetGMTerm(context.Background(), "Director")
	tests := []struct {
		name    string
		ctx     context.Context
		owner   bool
		want    []string
		notWant []string
	}{
		{"player", base, false, []string{"Timelines your Director shares will show here", "Nothing to do yet"}, []string{"Add a timeline", "New Timeline", "Create", "create-timeline-modal"}},
		{"co-DM", layouts.SetIsDmGranted(base, true), false, []string{"Timelines the owner adds"}, []string{"Add a timeline", "New Timeline", "create-timeline-modal"}},
		{"owner", layouts.SetIsOwner(base, true), true, []string{"Lay out events along a line", "Add a timeline", "create-timeline-modal"}, []string{"Nothing to do yet"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := timelineListContent(cc, TimelineListData{CampaignID: "c1", IsOwner: tt.owner}).Render(tt.ctx, &buf); err != nil {
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
					t.Errorf("a viewer who can't create timelines sees %q", w)
				}
			}
		})
	}
}

// With timelines the page is one title line with the count beside it.
func TestTimelineListOpening_WithTimelines(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}}
	var buf bytes.Buffer
	data := TimelineListData{CampaignID: "c1", Timelines: []Timeline{{ID: "t1", Name: "The war"}, {ID: "t2", Name: "The thaw"}}}
	if err := timelineListContent(cc, data).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `data-app-opening="full"`) || !strings.Contains(out, "2 timelines") || strings.Contains(out, "No timelines yet") {
		t.Errorf("unexpected opening:\n%s", out)
	}
}
