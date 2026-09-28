package app

// upcoming_events_block_test.go pins upcomingEventsBlockShell's two
// outcomes: the calendar addon unavailable for this campaign (off, or on but
// the plugin's schema is degraded) renders the honest "isn't enabled" note
// with no hx-get at all, and the normal case renders the lazy-load shell
// pointed at the calendar plugin's own role/viewer-gated
// GET /campaigns/:id/calendars/upcoming fragment — this block carries no
// event or calendar data of its own, so there is nothing here for a Player
// to see beyond what that already-viewer-gated fragment route decides to
// send back (see internal/plugins/calendar/upcoming_events_test.go for that
// route's own dm_only/hidden-calendar coverage).

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

func TestUpcomingEventsBlockShell_Unavailable_NoHxGet(t *testing.T) {
	// Neither addon-on-but-healthy fact was ever set on this ctx (the zero
	// value layouts.UpcomingEventsAvailable returns for an unset key), the
	// same "unknown = unavailable" default calendarCardUnavailable relies on.
	got := renderToString(t, upcomingEventsBlockShell("camp-1", 5))

	if strings.Contains(got, "hx-get") {
		t.Errorf("an unavailable calendar must render no hx-get at all, got %q", got)
	}
	if !strings.Contains(got, "isn't enabled") {
		t.Errorf("expected the honest unavailable note, got %q", got)
	}
}

func TestUpcomingEventsBlockShell_Available_RendersLazyFragment(t *testing.T) {
	ctx := layouts.SetUpcomingEventsAvailable(context.Background(), true)

	var buf strings.Builder
	if err := upcomingEventsBlockShell("camp-1", 7).Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, `hx-get="/campaigns/camp-1/calendars/upcoming?limit=7"`) {
		t.Errorf("expected the shared calendar upcoming-events fragment URL with this block's own limit, got %q", got)
	}
	if !strings.Contains(got, `hx-trigger="intersect once"`) {
		t.Errorf("expected a lazy intersect-once load (never eager), got %q", got)
	}
}

func TestUpcomingEventsBlockShell_ExplicitlyUnavailable_NoHxGet(t *testing.T) {
	ctx := layouts.SetUpcomingEventsAvailable(context.Background(), false)

	var buf strings.Builder
	if err := upcomingEventsBlockShell("camp-1", 5).Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()

	if strings.Contains(got, "hx-get") {
		t.Errorf("a degraded calendar plugin must render no hx-get (a 404'd fragment with no swap), got %q", got)
	}
}
