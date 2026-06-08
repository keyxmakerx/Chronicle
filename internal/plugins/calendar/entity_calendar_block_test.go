// entity_calendar_block_test.go — C-CAL-ENTITY-PAGE-EMBED. The entity-page
// calendar embed renders the band + the entity's linked events, filters
// dm_only by viewer role, and shows empty-but-present states (never blank).
package calendar

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// entityCalBlockStub satisfies CalendarService via embedding; only the three
// methods EntityCalendarBlock calls are overridden.
type entityCalBlockStub struct {
	CalendarService
	cal  *Calendar
	seed *WorldStateSeed
	ties []EntityEventTie
}

func (s *entityCalBlockStub) GetCalendar(context.Context, string) (*Calendar, error) {
	return s.cal, nil
}
func (s *entityCalBlockStub) BuildWorldStateSeed(context.Context, string, int, int, int, int, string) (*WorldStateSeed, error) {
	return s.seed, nil
}
func (s *entityCalBlockStub) EventsForEntity(context.Context, string) ([]EntityEventTie, error) {
	return s.ties, nil
}

func renderEntityCal(t *testing.T, svc CalendarService, role campaigns.Role, dmGranted bool) string {
	t.Helper()
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role, IsDmGranted: dmGranted}
	var sb strings.Builder
	if err := EntityCalendarBlock(svc, cc, "ent-1", "user-1", "", "").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

func sampleEmbedSvc() *entityCalBlockStub {
	return &entityCalBlockStub{
		cal:  &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Harptos", CurrentYear: 1492, CurrentMonth: 4, CurrentDay: 15, HoursPerDay: 24, MinutesPerHour: 60},
		seed: &WorldStateSeed{TimeOfDay: 0.5, Season: "Spring", Date: WorldStateDate{1492, 4, 15}, Weather: WorldStateWeather{Type: "rain", Intensity: 1}},
		ties: []EntityEventTie{
			{Event: Event{ID: "e1", Name: "Public Siege", Year: 1492, Month: 4, Day: 15, Visibility: "everyone"}, ParticipationRole: "involved"},
			{Event: Event{ID: "e2", Name: "Secret Pact", Year: 1492, Month: 4, Day: 16, Visibility: "dm_only"}, ParticipationRole: "mentioned"},
		},
	}
}

func TestEntityCalendarBlock_RendersBandAndEvents(t *testing.T) {
	html := renderEntityCal(t, sampleEmbedSvc(), campaigns.RoleOwner, false)
	for _, want := range []string{
		"data-entity-calendar",      // block container
		"id=\"cal-v2-worldstate\"",  // seed blob → engine prod mode
		"data-cal-sky",              // the reused 2a band scaffold
		"/static/js/cal-almanac.js", // the shared engine
		"Linked events",             // the events section header
		"Public Siege",              // the linked event
		"involved",                  // its participation role
	} {
		if !strings.Contains(html, want) {
			t.Errorf("embed missing %q", want)
		}
	}
}

func TestEntityCalendarBlock_DmOnlyFiltering(t *testing.T) {
	// Owner sees the secret event; Player does not.
	owner := renderEntityCal(t, sampleEmbedSvc(), campaigns.RoleOwner, false)
	if !strings.Contains(owner, "Secret Pact") {
		t.Errorf("owner should see the dm_only linked event")
	}
	player := renderEntityCal(t, sampleEmbedSvc(), campaigns.RolePlayer, false)
	if strings.Contains(player, "Secret Pact") {
		t.Errorf("player must NOT see the dm_only linked event")
	}
	if !strings.Contains(player, "Public Siege") {
		t.Errorf("player should still see the public linked event")
	}
	// A DM-granted player (co-DM) sees it.
	coDM := renderEntityCal(t, sampleEmbedSvc(), campaigns.RolePlayer, true)
	if !strings.Contains(coDM, "Secret Pact") {
		t.Errorf("co-DM (dm-granted) should see the dm_only linked event")
	}
}

// TestEntityCalendarBlock_Unavailable: no campaign context renders the friendly
// unavailable state, never a raw error/blank (item 2).
func TestEntityCalendarBlock_Unavailable(t *testing.T) {
	render := func(cc *campaigns.CampaignContext, entityID string) string {
		var sb strings.Builder
		if err := EntityCalendarBlock(sampleEmbedSvc(), cc, entityID, "u1", "", "").Render(context.Background(), &sb); err != nil {
			t.Fatalf("render: %v", err)
		}
		return sb.String()
	}
	// No campaign context → friendly unavailable state, no band.
	html := render(nil, "ent-1")
	if !strings.Contains(html, "data-entity-calendar-unavailable") {
		t.Errorf("nil cc: expected friendly unavailable state, got: %q", html)
	}
	if strings.Contains(html, "data-cal-sky") {
		t.Errorf("nil cc: must not render the band")
	}
}

// TestEntityCalendarBlock_PreviewPlaceholder (C-WIDGET-BINDING-QA1 Bug 2): a
// concrete-entity block rendered WITHOUT an entity (customization/layout editor
// or preview) shows the CALM "previews on the entity page" placeholder — never
// the alarming can't-load copy — and never the band.
func TestEntityCalendarBlock_PreviewPlaceholder(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	var sb strings.Builder
	if err := EntityCalendarBlock(sampleEmbedSvc(), cc, "", "u1", "", "").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "data-entity-calendar-preview") {
		t.Errorf("empty entity: expected calm preview placeholder, got: %q", html)
	}
	// Must NOT be the alarming unavailable copy, and must not render the band.
	if strings.Contains(html, "data-entity-calendar-unavailable") || strings.Contains(html, "data-cal-sky") {
		t.Errorf("empty entity: must show the preview placeholder only, got: %q", html)
	}
}

func TestEntityCalendarBlock_EmptyStates(t *testing.T) {
	// No calendar → friendly "Create calendar" CTA (C-CAL-EMBED-CONVERGE-POLISH
	// item 3), not a raw message; no band.
	noCal := renderEntityCal(t, &entityCalBlockStub{}, campaigns.RoleOwner, false)
	for _, want := range []string{"data-entity-calendar-empty", "No calendar yet", "Create calendar", "/campaigns/camp-1/calendars"} {
		if !strings.Contains(noCal, want) {
			t.Errorf("no-calendar empty state missing %q; got: %q", want, noCal)
		}
	}
	if strings.Contains(noCal, "data-cal-sky") {
		t.Errorf("no-calendar should not render the band")
	}
	// Calendar but no linked events → header + "No linked events".
	svc := sampleEmbedSvc()
	svc.ties = nil
	noTies := renderEntityCal(t, svc, campaigns.RoleOwner, false)
	if !strings.Contains(noTies, "Linked events") || !strings.Contains(noTies, "No linked events") {
		t.Errorf("no-ties should show the header + empty note, got: %q", noTies)
	}
}
