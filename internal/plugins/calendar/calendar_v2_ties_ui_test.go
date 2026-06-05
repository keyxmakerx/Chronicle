// calendar_v2_ties_ui_test.go — C-CAL-WORLDSTATE-PRODUCTION-PORT 2b.
// The event drawer carries the attach-entity picker scaffold, and the role
// vocabulary it advertises is sourced from the Go ParticipationRoles enum
// (so the showcase R3 picker + the backend + this UI all share one list).
package calendar

import (
	"context"
	"strings"
	"testing"
)

func TestEventDrawer_HasEntityPicker(t *testing.T) {
	data := CalendarV2ViewData{
		ActiveCalendar: &Calendar{ID: "cal-1", Name: "Harptos", Months: []Month{{Name: "Hammer", Days: 30}}},
		CampaignID:     "camp-1",
	}
	var sb strings.Builder
	if err := eventV2Drawer(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render drawer: %v", err)
	}
	html := sb.String()
	for _, want := range []string{
		"data-event-ties-section", // the picker container
		"data-ties-list",          // attached chips
		"data-ties-search",        // entity search input
		"data-ties-roles=",        // the role vocab attribute
	} {
		if !strings.Contains(html, want) {
			t.Errorf("event drawer missing picker hook %q", want)
		}
	}
	// The advertised roles MUST be exactly the four pinned ParticipationRoles
	// (matches Phase 1.5 R3's picker + #402's enum) — one source of truth.
	if !strings.Contains(html, `data-ties-roles="involved,present,affected,mentioned"`) {
		t.Errorf("picker role vocab must equal the ParticipationRoles enum")
	}
}

func TestParticipationRolesCSV(t *testing.T) {
	if got := participationRolesCSV(); got != "involved,present,affected,mentioned" {
		t.Errorf("participationRolesCSV = %q", got)
	}
}
