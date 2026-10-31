package sessions

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestSessionPageHasNoScriptInsideTheSwappedRegion pins that the session
// page's edit dialog works after a boosted navigation: htmx removes every
// script tag from swapped content (allowScriptTags=false), so Save is wired
// by the session-edit widget, and the status starts selected in the markup.
func TestSessionPageHasNoScriptInsideTheSwappedRegion(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp1", Name: "Test Campaign"}}
	s := &Session{ID: "s1", CampaignID: "camp1", Name: "Night one", Status: "completed"}
	var sb strings.Builder
	if err := SessionDetailPage(cc, s, "tok", true, true, "u1", RecapView{}, "UTC").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	page := sb.String()
	at := strings.Index(page, `id="main-content"`)
	if at < 0 {
		t.Fatal(`no id="main-content" in the rendered page`)
	}
	main := page[at:]
	if end := strings.Index(main, "</main>"); end >= 0 {
		main = main[:end]
	}
	if n := strings.Count(main, "<script"); n != 0 {
		t.Errorf("the session page emits %d script tag(s) inside #main-content", n)
	}
	if !strings.Contains(main, `data-widget="session-edit"`) {
		t.Error("the edit dialog is not a session-edit mount")
	}
	if !strings.Contains(main, `<option value="completed" selected>`) {
		t.Error("the stored status is not selected in the markup")
	}
}
