// dashboard_calendar_card_test.go pins the fix for the calendar dashboard
// cards' HTMX-swap trap: when the calendar addon is off, or its plugin is
// degraded (schema unhealthy, its routes never registered), the card must
// render a quiet neutral state — no hx-get, no stuck "Loading..." spinner,
// no error toast for every viewer — rather than emit an hx-get that 403s or
// 404s and can never swap.
package campaigns

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

func calendarCardTestCampaign() *CampaignContext {
	return &CampaignContext{
		Campaign:   &Campaign{ID: "camp-1"},
		MemberRole: RolePlayer,
		IsMember:   true,
	}
}

// calendarCardTestOwner is the owner, the one viewer still shown a calendar
// card while the calendar is off, so they know to turn it on.
func calendarCardTestOwner() *CampaignContext {
	cc := calendarCardTestCampaign()
	cc.MemberRole = RoleOwner
	return cc
}

// renderCardWithCtx renders c against ctx (carrying whatever layouts
// context values the test set up) and returns the HTML.
func renderCardWithCtx(t *testing.T, ctx context.Context, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestDashCalendarPreview_AddonDisabled_NoHxGetNoSpinner(t *testing.T) {
	cc := calendarCardTestOwner()
	ctx := layouts.SetUpcomingEventsAvailable(context.Background(), false)

	out := renderCardWithCtx(t, ctx, dashCalendarPreview(cc, nil))
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

func TestDashCalendarPreview_PluginUnhealthy_NoHxGetNoSpinner(t *testing.T) {
	cc := calendarCardTestOwner()
	// Addon-on-but-plugin-degraded is resolved to the same neutral flag as
	// addon-off before this template ever sees it (internal/app/routes.go);
	// from here the two scenarios are indistinguishable by design, since
	// this package may not read the calendar plugin's health itself.
	ctx := layouts.SetUpcomingEventsAvailable(context.Background(), false)

	out := renderCardWithCtx(t, ctx, dashCalendarPreview(cc, nil))
	if strings.Contains(out, "hx-get") {
		t.Errorf("a degraded calendar plugin (routes never registered) must not get an hx-get card: %s", out)
	}
	if !strings.Contains(out, "Calendar isn't enabled") {
		t.Errorf("expected the quiet neutral state, got: %s", out)
	}
}

func TestDashCalendarPreview_AddonEnabledAndHealthy_EmitsHxGet(t *testing.T) {
	cc := calendarCardTestCampaign()
	ctx := layouts.SetUpcomingEventsAvailable(context.Background(), true)

	out := renderCardWithCtx(t, ctx, dashCalendarPreview(cc, nil))
	if !strings.Contains(out, "hx-get") {
		t.Errorf("an enabled, healthy calendar addon must still emit its hx-get card: %s", out)
	}
	if !strings.Contains(out, "/calendars/upcoming") {
		t.Errorf("expected the upcoming-events endpoint in the hx-get target: %s", out)
	}
}

// TestDashCalendarPreview_NoContextSet_FailsToQuietState covers the ordinary
// safety default: a render whose context never had
// SetUpcomingEventsAvailable called at all (e.g. a test harness, or a
// future caller that forgets to wire it) must fail toward the quiet state,
// not toward silently assuming the addon is on — UpcomingEventsAvailable
// already defaults to false when unset.
func TestDashCalendarPreview_NoContextSet_FailsToQuietState(t *testing.T) {
	cc := calendarCardTestCampaign()
	out := renderCardWithCtx(t, context.Background(), dashCalendarPreview(cc, nil))
	if strings.Contains(out, "hx-get") {
		t.Errorf("an unset addon context must not be treated as enabled: %s", out)
	}
}

func TestDashCalendarFull_AddonDisabled_NoHxGetNoSpinner(t *testing.T) {
	cc := calendarCardTestOwner()
	ctx := layouts.SetUpcomingEventsAvailable(context.Background(), false)

	out := renderCardWithCtx(t, ctx, dashCalendarFull(cc, nil))
	if strings.Contains(out, "hx-get") || strings.Contains(out, "Loading...") {
		t.Errorf("addon-disabled full card must not emit hx-get or a stuck spinner: %s", out)
	}
	if !strings.Contains(out, "Calendar isn't enabled") {
		t.Errorf("expected the quiet neutral state: %s", out)
	}
}

// With the calendar off, anyone but the owner is shown no card at all: they
// can't turn it on, and "isn't enabled" over an empty card tells them nothing.
func TestDashCalendarCards_AddonDisabled_HiddenFromNonOwners(t *testing.T) {
	ctx := layouts.SetUpcomingEventsAvailable(context.Background(), false)
	for _, role := range []Role{RoleNone, RolePlayer, RoleScribe} {
		cc := calendarCardTestCampaign()
		cc.MemberRole = role
		for name, c := range map[string]templ.Component{
			"preview": dashCalendarPreview(cc, nil),
			"full":    dashCalendarFull(cc, nil),
		} {
			if out := strings.TrimSpace(renderCardWithCtx(t, ctx, c)); out != "" {
				t.Errorf("role %d, %s card: want nothing, got %s", role, name, out)
			}
		}
	}
}

// The calendar app is members-only, so a visitor's card links nowhere into it,
// and its lazy slot is marked to drop the card when nothing comes back.
func TestDashCalendarCards_VisitorGetsNoAppLink(t *testing.T) {
	ctx := layouts.SetUpcomingEventsAvailable(context.Background(), true)
	tests := []struct {
		name     string
		isMember bool
		wantLink bool
	}{
		{"member", true, true},
		{"visitor", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := calendarCardTestCampaign()
			cc.IsMember = tt.isMember
			if !tt.isMember {
				cc.MemberRole = RoleNone
			}
			for _, c := range []templ.Component{dashCalendarPreview(cc, nil), dashCalendarFull(cc, nil)} {
				out := renderCardWithCtx(t, ctx, c)
				if got := strings.Contains(out, "/apps/calendar"); got != tt.wantLink {
					t.Errorf("link to the calendar app = %v, want %v: %s", got, tt.wantLink, out)
				}
				if !strings.Contains(out, "data-drop-if-empty") || !strings.Contains(out, `data-block="upcoming"`) {
					t.Errorf("lazy slot must be marked to drop its card when empty: %s", out)
				}
			}
		})
	}
}
