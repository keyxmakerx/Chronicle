package timeline

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestTimelinePageHasNoScriptInsideTheSwappedRegion pins that the time line
// page survives a boosted sidebar navigation: htmx removes every script tag
// from a swapped fragment (allowScriptTags=false), so the page's Alpine
// components load on sight from the timeline-page mount instead.
func TestTimelinePageHasNoScriptInsideTheSwappedRegion(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp1", Name: "Test Campaign"}}
	data := TimelineViewData{
		CampaignID: "camp1",
		Timeline:   &Timeline{ID: "tl1", CampaignID: "camp1", Name: "Ages", Visibility: "everyone"},
		IsOwner:    true,
		IsScribe:   true,
	}
	var sb strings.Builder
	if err := TimelineShowFragment(cc, data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := sb.String()
	if n := strings.Count(out, "<script"); n != 0 {
		t.Errorf("the time line page emits %d script tag(s) in its swapped content", n)
	}
	if !strings.Contains(out, `data-widget="timeline-page" x-data x-ignore`) {
		t.Error("the page is not wrapped in the timeline-page mount that holds Alpine off until its components load")
	}
	// The visibility follows the ids directly: an extra argument there once
	// put the wrong value in the visibility panel.
	if !strings.Contains(out, "visibilityManager(&#39;camp1&#39;, &#39;tl1&#39;, &#39;everyone&#39;, null)") {
		t.Error("visibilityManager is not called with (campaign, timeline, visibility, rules)")
	}
}
