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
