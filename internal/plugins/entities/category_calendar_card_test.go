// category_calendar_card_test.go pins the category-dashboard twin of
// campaigns/dashboard_calendar_card_test.go's fix: catCalendarPreview must
// skip its hx-get entirely (a quiet neutral state instead) when the
// calendar addon is off or its plugin is degraded, rather than emit an
// hx-get that 403s/404s into a permanently stuck "Loading..." card.
package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

func renderCatCardWithCtx(t *testing.T, ctx context.Context, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestCatCalendarPreview_AddonDisabled_NoHxGetNoSpinner(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}}
	ctx := layouts.SetUpcomingEventsAvailable(context.Background(), false)

	out := renderCatCardWithCtx(t, ctx, catCalendarPreview(cc, nil))
	if strings.Contains(out, "hx-get") {
		t.Errorf("addon-disabled card must not emit hx-get: %s", out)
	}
	if strings.Contains(out, "Loading...") {
		t.Errorf("addon-disabled card must not show a permanent spinner: %s", out)
	}
	if !strings.Contains(out, "Calendar isn't enabled") {
		t.Errorf("addon-disabled card must show a quiet neutral message: %s", out)
	}
}

func TestCatCalendarPreview_PluginUnhealthy_NoHxGetNoSpinner(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}}
	// Addon-on-but-plugin-degraded is resolved to the same neutral flag as
	// addon-off before this template ever sees it (internal/app/routes.go);
	// from here the two scenarios are indistinguishable by design, since
	// this package may not read the calendar plugin's health itself.
	ctx := layouts.SetUpcomingEventsAvailable(context.Background(), false)

	out := renderCatCardWithCtx(t, ctx, catCalendarPreview(cc, nil))
	if strings.Contains(out, "hx-get") {
		t.Errorf("a degraded calendar plugin must not get an hx-get card: %s", out)
	}
	if !strings.Contains(out, "Calendar isn't enabled") {
		t.Errorf("expected the quiet neutral state, got: %s", out)
	}
}

// TestCatCalendarPreview_NoContextSet_FailsToQuietState mirrors
// campaigns.TestDashCalendarPreview_NoContextSet_FailsToQuietState: a render
// context that never had SetUpcomingEventsAvailable called at all must fail
// toward the quiet state, never toward assuming the addon is on.
func TestCatCalendarPreview_NoContextSet_FailsToQuietState(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}}
	out := renderCatCardWithCtx(t, context.Background(), catCalendarPreview(cc, nil))
	if strings.Contains(out, "hx-get") {
		t.Errorf("an unset addon context must not be treated as enabled: %s", out)
	}
}

func TestCatCalendarPreview_AddonEnabledAndHealthy_EmitsHxGet(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}}
	ctx := layouts.SetUpcomingEventsAvailable(context.Background(), true)

	out := renderCatCardWithCtx(t, ctx, catCalendarPreview(cc, nil))
	if !strings.Contains(out, "hx-get") {
		t.Errorf("an enabled, healthy calendar addon must still emit its hx-get card: %s", out)
	}
	if !strings.Contains(out, "/calendars/upcoming") {
		t.Errorf("expected the upcoming-events endpoint in the hx-get target: %s", out)
	}
}
