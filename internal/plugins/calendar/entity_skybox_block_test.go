// entity_skybox_block_test.go — C-SKYBOX-WIDGET. The sky-only ambient block
// renders the seeded skybox widget mount (no hourglass, unlike its
// entity_worldstate sibling), shows a friendly "Create calendar" CTA when
// there's no calendar, and a friendly unavailable state when there's no
// entity/campaign context (never a raw error / blank). Visual fidelity is
// the operator's local gate.
package calendar

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// entitySkyboxBlockStub satisfies CalendarService via embedding; only the
// two methods EntitySkyboxBlock calls are overridden.
type entitySkyboxBlockStub struct {
	CalendarService
	cal  *Calendar
	seed *WorldStateSeed
}

func (s *entitySkyboxBlockStub) GetCalendar(context.Context, string) (*Calendar, error) {
	return s.cal, nil
}
func (s *entitySkyboxBlockStub) BuildWorldStateSeed(context.Context, string, int, int, int, int, string) (*WorldStateSeed, error) {
	return s.seed, nil
}

func renderEntitySkybox(t *testing.T, svc CalendarService, cc *campaigns.CampaignContext, entityID string) string {
	t.Helper()
	var sb strings.Builder
	if err := EntitySkyboxBlock(svc, cc, entityID, "user-1").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

func sampleSkyboxSvc() *entitySkyboxBlockStub {
	return &entitySkyboxBlockStub{
		cal:  &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Harptos", CurrentYear: 1492, CurrentMonth: 4, CurrentDay: 15, HoursPerDay: 24, MinutesPerHour: 60},
		seed: &WorldStateSeed{TimeOfDay: 0.5, Season: "Spring", Date: WorldStateDate{1492, 4, 15}, Weather: WorldStateWeather{Type: "rain", Intensity: 1}},
	}
}

func TestEntitySkyboxBlock_RendersWidgetMount(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	html := renderEntitySkybox(t, sampleSkyboxSvc(), cc, "")
	for _, want := range []string{
		"data-entity-skybox",                        // block container
		`data-widget="skybox"`,                      // boot.js mount point
		`data-campaign-id="camp-1"`,                 // provider key
		"id=\"cal-v2-worldstate-skybox-\"",          // per-block namespaced seed id (no duplicate-id collisions)
		"data-cal-worldstate=",                      // the seed blob the engine reads by attribute
		"data-cal-sky",                              // the reused sky scaffold
		"data-skybox-error",                         // friendly client error target
		"/static/js/widgets/worldstate_provider.js", // the SAME provider singleton the worldstate widget uses
		"/static/js/widgets/skybox.js",              // the widget
		"/static/js/cal-almanac.js",                 // the shared engine
	} {
		if !strings.Contains(html, want) {
			t.Errorf("skybox embed missing %q", want)
		}
	}
	// Sky-only: never the hourglass shelf.
	if strings.Contains(html, "cal-almanac-shelf") {
		t.Errorf("skybox block must not render the hourglass shelf (that's entity_worldstate)")
	}
}

func TestEntitySkyboxBlock_EmptyState(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	html := renderEntitySkybox(t, &entitySkyboxBlockStub{}, cc, "") // no calendar
	for _, want := range []string{"data-entity-skybox-empty", "No calendar yet", "Create calendar", "/campaigns/camp-1/calendars"} {
		if !strings.Contains(html, want) {
			t.Errorf("no-calendar empty state missing %q; got: %q", want, html)
		}
	}
	if strings.Contains(html, "data-cal-sky") || strings.Contains(html, `data-widget="skybox"`) {
		t.Errorf("no-calendar state must not render the band / widget mount")
	}
}

// Campaign-level: the block renders without an entity (dashboard context) as
// long as the campaign + calendar are present — same reasoning as
// entity_worldstate (dispatch decision D), just without per-entity binding.
func TestEntitySkyboxBlock_NoEntityStillRenders(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	html := renderEntitySkybox(t, sampleSkyboxSvc(), cc, "")
	if !strings.Contains(html, `data-widget="skybox"`) {
		t.Errorf("skybox block should render campaign-level without an entity")
	}
}

// Two entity ids must not collide on the same seed-blob element id.
func TestEntitySkyboxBlock_NamespacesSeedIDPerEntity(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	htmlA := renderEntitySkybox(t, sampleSkyboxSvc(), cc, "ent-a")
	htmlB := renderEntitySkybox(t, sampleSkyboxSvc(), cc, "ent-b")
	if !strings.Contains(htmlA, `id="cal-v2-worldstate-skybox-ent-a"`) {
		t.Errorf("entity A should get its own namespaced seed id; got: %q", htmlA)
	}
	if !strings.Contains(htmlB, `id="cal-v2-worldstate-skybox-ent-b"`) {
		t.Errorf("entity B should get its own namespaced seed id; got: %q", htmlB)
	}
}

func TestEntitySkyboxBlock_Unavailable(t *testing.T) {
	// No campaign context → friendly unavailable state (never a raw error).
	html := renderEntitySkybox(t, sampleSkyboxSvc(), nil, "")
	if !strings.Contains(html, "data-entity-skybox-unavailable") {
		t.Errorf("nil cc: expected friendly unavailable state, got: %q", html)
	}
	if strings.Contains(html, "data-cal-sky") {
		t.Errorf("nil cc: must not render the band")
	}
}

// R2 crash-guard parity (same class as TestWorldStateSkyBandV2_NilWorldStateNoPanic):
// a transient BuildWorldStateSeed error must not panic ranging a nil WorldState.Events.
func TestEntitySkyboxBlock_SeedBuildErrorNoPanic(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	svc := &entitySkyboxBlockStub{cal: sampleSkyboxSvc().cal, seed: nil} // BuildWorldStateSeed "succeeds" with a nil seed
	html := renderEntitySkybox(t, svc, cc, "")
	if !strings.Contains(html, "data-cal-sky") {
		t.Errorf("band should still render its sky scaffold with a nil seed")
	}
}
