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
	if err := SessionDetailPage(cc, s, "tok", true, true, "u1", RecapView{}, "UTC", NightDateNote{}).Render(context.Background(), &sb); err != nil {
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

// TestNightDateCardIsAWidgetMount pins that the Real date card on a date-less
// night's page is a night-date mount with no inline handler and no script
// inside the swapped region, so it works after a boosted navigation.
func TestNightDateCardIsAWidgetMount(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp1", Name: "Test Campaign"}}
	s := &Session{ID: "s1", CampaignID: "camp1", Name: "Night one", Status: "planned"}
	note := NightDateNote{NoRealDate: true, Words: NoRealDateWords}
	var sb strings.Builder
	if err := SessionDetailPage(cc, s, "tok", true, true, "u1", RecapView{}, "UTC", note).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	page := sb.String()
	at := strings.Index(page, `data-testid="set-real-date"`)
	if at < 0 {
		t.Fatal("no Real date card on a date-less night's page for a Scribe")
	}
	card := page[at:]
	if end := strings.Index(card, "Set the date"); end >= 0 {
		card = card[:end]
	}
	for _, want := range []string{`data-widget="night-date"`, `id="night-real-date"`, `id="night-real-date-btn"`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if strings.Contains(card, "onclick") || strings.Contains(card, "<script") {
		t.Error("the Real date card has an inline handler or script tag")
	}
}
