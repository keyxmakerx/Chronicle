package calendar

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// An empty list opens large with the sentence for the viewer, a non-owner is
// never told there are none, and only the owner is offered the wizard.
func TestCalendarsList_Opening(t *testing.T) {
	render := func(ctx context.Context, d CalendarsListData) string {
		var buf bytes.Buffer
		if err := CalendarsListFragment(d).Render(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	owner := render(layouts.SetIsOwner(context.Background(), true), CalendarsListData{CampaignID: "c1", IsOwner: true})
	if !strings.Contains(owner, `data-app-opening="empty"`) || !strings.Contains(owner, "New calendar") || !strings.Contains(owner, "Import") {
		t.Errorf("owner should see the large opening with New calendar and Import:\n%s", owner)
	}
	player := render(context.Background(), CalendarsListData{CampaignID: "c1"})
	if !strings.Contains(player, "shares a calendar with you") || strings.Contains(player, "New calendar") || strings.Contains(player, "No calendars") {
		t.Errorf("player should see only the sharing sentence:\n%s", player)
	}
}

// templ refuses an on* attribute built from a plain string and prints a
// comment in its place. "New calendar" records its button for the wizard to
// return focus to through a data attribute instead.
func TestCalendarsList_ActionIsNotDead(t *testing.T) {
	var buf bytes.Buffer
	ctx := layouts.SetIsOwner(context.Background(), true)
	if err := CalendarsListFragment(CalendarsListData{CampaignID: "c1", IsOwner: true}).Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "failed sanitization") {
		t.Errorf("a button carries an attribute templ refused to print:\n%s", out)
	}
	if !strings.Contains(out, `data-opener-global="__calv5Opener"`) {
		t.Errorf("New calendar does not record its opener:\n%s", out)
	}
}
