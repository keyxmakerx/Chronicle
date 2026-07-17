// skybox_test.go covers the Skybox widget's own render contract: the
// data-widget mount + seed blob + happening chips, and that it stays
// nil-safe with an empty/zero-value SkyboxData (mirrors
// TestWorldStateSkyBandV2_NilWorldStateNoPanic in the calendar plugin,
// which now exercises this package indirectly via delegation).
package skybox

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func render(t *testing.T, data SkyboxData, sunBody templ.Component) string {
	t.Helper()
	var sb strings.Builder
	if err := Skybox(data, sunBody).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render skybox: %v", err)
	}
	return sb.String()
}

func TestSkybox_RendersMountAndSeedBlob(t *testing.T) {
	html := render(t, SkyboxData{
		ElementID:  "cal-v2-worldstate",
		CampaignID: "camp-1",
		SeedJSON:   `{"timeOfDay":0.5}`,
		WeatherID:  "rain",
		Gradient:   "linear-gradient(180deg, red, blue)",
	}, templ.NopComponent)
	for _, want := range []string{
		`data-widget="skybox"`,
		`data-campaign-id="camp-1"`,
		`data-cal-sky`,
		`data-cal-sky-weather="rain"`,
		`cal-almanac-sky--wfx-rain`,
		`id="cal-v2-worldstate"`,
		`data-cal-worldstate="{&#34;timeOfDay&#34;:0.5}"`,
		`data-cal-sky-canvas`,
		`data-cal-sky-canvas-front`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
}

func TestSkybox_EmptyDataDoesNotPanic(t *testing.T) {
	html := render(t, SkyboxData{}, templ.NopComponent)
	if !strings.Contains(html, "data-cal-sky") {
		t.Errorf("band should still render its sky scaffold with zero-value data")
	}
}

func TestSkybox_RendersEventHappeningChips(t *testing.T) {
	html := render(t, SkyboxData{
		Events: []SkyboxEvent{{Name: "Meteor Shower"}, {Name: "Solar Eclipse"}},
	}, templ.NopComponent)
	if strings.Count(html, "cal-almanac-sky__happening-chip") != 2 {
		t.Errorf("expected 2 happening chips, got html:\n%s", html)
	}
	for _, want := range []string{`title="Meteor Shower"`, `title="Solar Eclipse"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing chip title %q", want)
		}
	}
}

func TestSkybox_NoEventsRendersEmptyHappeningTray(t *testing.T) {
	html := render(t, SkyboxData{}, templ.NopComponent)
	if strings.Contains(html, "cal-almanac-sky__happening-chip") {
		t.Errorf("no events should mean no happening chips")
	}
	if !strings.Contains(html, `data-cal-sky-happening`) {
		t.Errorf("the (empty) happening tray container should still render")
	}
}

func TestSkybox_RendersInjectedSunBody(t *testing.T) {
	sun := templ.Raw(`<span class="cal-almanac-sun__rays"></span>`)
	html := render(t, SkyboxData{}, sun)
	if !strings.Contains(html, "cal-almanac-sun__rays") {
		t.Errorf("sunBody child component should render; got:\n%s", html)
	}
}
