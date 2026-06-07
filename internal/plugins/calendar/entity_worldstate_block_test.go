// entity_worldstate_block_test.go — C-CAL-WORLDSTATE-WIDGETS. The entity-page
// worldState timepiece embed renders the seeded band + hourglass-shelf widget
// mount, shows a friendly "Create calendar" CTA when there's no calendar, and
// a friendly unavailable state when there's no entity/campaign context (never
// a raw error / blank). Visual fidelity is the operator's local gate.
package calendar

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// entityWSBlockStub satisfies CalendarService via embedding; only the two
// methods EntityWorldStateBlock calls are overridden.
type entityWSBlockStub struct {
	CalendarService
	cal  *Calendar
	seed *WorldStateSeed
}

func (s *entityWSBlockStub) GetCalendar(context.Context, string) (*Calendar, error) {
	return s.cal, nil
}
func (s *entityWSBlockStub) BuildWorldStateSeed(context.Context, string, int, int, int, int, string) (*WorldStateSeed, error) {
	return s.seed, nil
}

func renderEntityWS(t *testing.T, svc CalendarService, cc *campaigns.CampaignContext) string {
	t.Helper()
	var sb strings.Builder
	if err := EntityWorldStateBlock(svc, cc, "user-1", "").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

func sampleWSSvc() *entityWSBlockStub {
	return &entityWSBlockStub{
		cal:  &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Harptos", CurrentYear: 1492, CurrentMonth: 4, CurrentDay: 15, HoursPerDay: 24, MinutesPerHour: 60},
		seed: &WorldStateSeed{TimeOfDay: 0.5, Season: "Spring", Date: WorldStateDate{1492, 4, 15}, Weather: WorldStateWeather{Type: "rain", Intensity: 1}},
	}
}

func TestEntityWorldStateBlock_RendersWidgetMount(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	html := renderEntityWS(t, sampleWSSvc(), cc)
	for _, want := range []string{
		"data-entity-worldstate",                       // block container
		`data-widget="worldstate"`,                     // boot.js mount point
		`data-variant="hourglass"`,                     // the mini-shelf variant
		`data-campaign-id="camp-1"`,                    // provider key
		"id=\"cal-v2-worldstate\"",                     // seed blob → engine prod + provider zero-fetch
		"data-cal-sky",                                 // the reused sky scaffold
		"cal-almanac-shelf",                            // the hourglass-on-shelf
		"data-worldstate-error",                        // friendly client error target
		"/static/js/widgets/worldstate_provider.js",    // the provider singleton
		"/static/js/widgets/worldstate.js",             // the widget
		"/static/js/cal-almanac.js",                    // the shared engine
	} {
		if !strings.Contains(html, want) {
			t.Errorf("worldstate embed missing %q", want)
		}
	}
}

func TestEntityWorldStateBlock_EmptyState(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	html := renderEntityWS(t, &entityWSBlockStub{}, cc) // no calendar
	for _, want := range []string{"data-entity-worldstate-empty", "No calendar yet", "Create calendar", "/campaigns/camp-1/calendars"} {
		if !strings.Contains(html, want) {
			t.Errorf("no-calendar empty state missing %q; got: %q", want, html)
		}
	}
	if strings.Contains(html, "data-cal-sky") || strings.Contains(html, `data-widget="worldstate"`) {
		t.Errorf("no-calendar state must not render the band / widget mount")
	}
}

// Campaign-level: the block renders without an entity (dashboard context) as
// long as the campaign + calendar are present (dispatch decision D).
func TestEntityWorldStateBlock_NoEntityStillRenders(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	html := renderEntityWS(t, sampleWSSvc(), cc)
	if !strings.Contains(html, `data-widget="worldstate"`) {
		t.Errorf("worldstate block should render campaign-level without an entity")
	}
}

func TestEntityWorldStateBlock_Unavailable(t *testing.T) {
	// No campaign context → friendly unavailable state (never a raw error).
	html := renderEntityWS(t, sampleWSSvc(), nil)
	if !strings.Contains(html, "data-entity-worldstate-unavailable") {
		t.Errorf("nil cc: expected friendly unavailable state, got: %q", html)
	}
	if strings.Contains(html, "data-cal-sky") {
		t.Errorf("nil cc: must not render the band")
	}
}
