// calendar_v2_accent_pill_test.go — C-TZ-CONSOLIDATION rider 1 (#541
// deviation-3 booked follow-up). calendarV2ViewPill's active branch already
// reads the App-accent slot (shipped in 9d3b7954, C-CAL-SKY-STRIP, as a
// drive-by of an unrelated dispatch) — this file adds the byte-identity-style
// pin that follow-up was supposed to ship with, so a future edit can't
// silently drop the App-accent-first fallback chain without failing a test.
//
// The pin is a plain substring assertion, not a context-driven render (unlike
// data_accent_test.go's AccentColorCSS tests): calendarV2ViewPill emits a
// STATIC style string every time — the App→surface-1→site fallback is
// resolved by the BROWSER via CSS var() cascading, not by Go per campaign.
// That's exactly why the zero-change guarantee holds for campaigns that never
// set an App accent: the emitted DOM is identical regardless of context, and
// the CSS variable itself is simply unset, so var() falls through.
package calendar

import (
	"context"
	"strings"
	"testing"
)

// wantActivePillStyle is the exact fallback chain the active view pill must
// keep emitting: App accent first, then the legacy surface-1 accent
// (C-ACCENT-TRIO), then the site accent, then a hardcoded default — pinned
// per the accent-slot mapping (cordinator decisions/2026-05-21-core-tenets.md
// is silent on accents specifically; the binding mapping is
// cordinator/plans/2026-07-13-calendar-design-notes.md §"Operator feedback
// round 1", 1=site/2=action/3+4=app).
const wantActivePillStyle = `style="background: var(--color-accent-app, var(--color-accent-surface-1, var(--color-accent, #6366f1)));"`

// TestCalendarV2ViewPill_ActiveUsesAppAccentSlot pins that the active
// Month/Week/Day/Timeline pill reads the App-accent slot (slot 3), not the
// site accent — the rider booked at C-ACCENT-SLOTS #541 deviation-3 (deferred
// there because calendar_v2.templ was out of scope that wave).
func TestCalendarV2ViewPill_ActiveUsesAppAccentSlot(t *testing.T) {
	var sb strings.Builder
	data := designPass1Data("month", nil)
	if err := calendarV2ViewPill(data, "month", "Month").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render active pill: %v", err)
	}
	got := sb.String()
	if !strings.Contains(got, wantActivePillStyle) {
		t.Errorf("active view pill style = %q, want it to contain %q", got, wantActivePillStyle)
	}
	if strings.Contains(got, "text-accent") {
		t.Errorf("active view pill must not fall back to the bare site-accent utility class: %q", got)
	}
}

// TestCalendarV2ViewPill_InactiveCarriesNoAccentStyle pins the inverse: an
// inactive pill is a plain link with no inline accent style at all (it only
// gets hover treatment via the shared `hover:bg-surface-2` utility) — so the
// App-accent slot is visually exclusive to the active tab, not bleeding into
// the whole switcher.
func TestCalendarV2ViewPill_InactiveCarriesNoAccentStyle(t *testing.T) {
	var sb strings.Builder
	data := designPass1Data("month", nil) // active view = month
	if err := calendarV2ViewPill(data, "week", "Week").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render inactive pill: %v", err)
	}
	got := sb.String()
	if strings.Contains(got, "--color-accent-app") {
		t.Errorf("inactive pill must not carry the App-accent style: %q", got)
	}
	if !strings.Contains(got, `aria-selected="false"`) {
		t.Errorf("inactive pill must be aria-selected=false: %q", got)
	}
}

// TestCalendarV2ViewPill_ZeroChangeForUnsetAppAccent is the byte-identity
// pin: campaigns that never set a custom App accent render the EXACT SAME
// active-pill markup regardless of what (if anything) is in the render
// context — because the fallback resolution happens client-side via CSS
// var(), Go never branches on GetAccentApp here. This guards against a future
// "helpful" refactor that makes the style context-dependent and accidentally
// breaks campaigns with no App accent set.
func TestCalendarV2ViewPill_ZeroChangeForUnsetAppAccent(t *testing.T) {
	render := func(ctx context.Context) string {
		var sb strings.Builder
		data := designPass1Data("month", nil)
		if err := calendarV2ViewPill(data, "month", "Month").Render(ctx, &sb); err != nil {
			t.Fatalf("render: %v", err)
		}
		return sb.String()
	}

	bare := render(context.Background())
	if !strings.Contains(bare, wantActivePillStyle) {
		t.Fatalf("unset-context render = %q, want the fallback-chain style", bare)
	}
	// Unrelated context values (no accent set at all) must not change the
	// emitted pill markup — it's the same static string either way.
	withUnrelatedCtx := render(context.WithValue(context.Background(), struct{ k string }{"unrelated"}, "x"))
	if bare != withUnrelatedCtx {
		t.Errorf("active pill markup drifted with an unrelated context value:\n got: %q\nwant: %q", withUnrelatedCtx, bare)
	}
}
