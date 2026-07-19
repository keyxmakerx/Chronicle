// calendar_v2_skypane_detach_test.go — C-CAL-SKYPANE-DETACH.
//
// Page-level render guards for the skypane detach: the always-on worldState
// band is gone from CalendarV2Page, the desktop mini-month sidebar is detached,
// and the GM world-state console is re-homed as the skypane pull-out card
// (still capability-gated). The skybox engine seed + assets the band used to
// load now ride the sky strip, so the strip's skybox + the GM console's live
// re-render keep working after the band's removal.
package calendar

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// skypaneDetachPageData builds a fully-populated Month-view page payload with a
// worldState seed (so wsActive → the relocated engine assets render) for a
// capability holder by default.
func skypaneDetachPageData(canControl bool) CalendarV2ViewData {
	cal := gmTestCalendar() // 12 months, real-time-ish fantasy calendar
	tint := "#c8d8ff"
	seed := &WorldStateSeed{
		TimeOfDay: 0.5,
		Season:    "Spring",
		Date:      WorldStateDate{Year: cal.CurrentYear, Month: cal.CurrentMonth, Day: cal.CurrentDay},
		Moons: []WorldStateMoon{
			{ID: 1, Name: "Selune", BaseDesign: "moon-realistic-selene", Tint: &tint, PhaseSource: "css-clip", Size: 1, OrbitSpeed: 1, CyclePct: 0.5, NamedPhase: "Full", NamedPhases: []WorldStateMoonPhase{}},
		},
		Weather: WorldStateWeather{Type: "rain", Intensity: 1},
		Events:  []WorldStateEvent{{Type: "meteor-shower", Name: "Tears of Selune", StartTime: 22, Duration: 4}},
	}
	return CalendarV2ViewData{
		ActiveCalendar:       cal,
		AllCalendars:         []Calendar{*cal},
		View:                 "month",
		Year:                 cal.CurrentYear,
		Month:                cal.CurrentMonth,
		Day:                  cal.CurrentDay,
		CampaignID:           "camp-1",
		UserID:               "user-1",
		IsOwner:              canControl,
		IsScribe:             canControl,
		CSRFToken:            "tok",
		SidebarPinned:        true,
		WorldState:           seed,
		WorldStateJSON:       `{"timeOfDay":0.5,"season":"Spring"}`,
		CanControlWorldState: canControl,
	}
}

func renderSkypanePage(t *testing.T, data CalendarV2ViewData) string {
	t.Helper()
	cc := &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1", Name: "The Sunless Reaches"},
		MemberRole: campaigns.RoleOwner,
		IsMember:   true,
	}
	var sb strings.Builder
	if err := CalendarV2Page(cc, data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render CalendarV2Page: %v", err)
	}
	return sb.String()
}

// TestCalendarV2Page_NoBand_NoDesktopSidebar_ConsoleInSkypane: the three
// acceptance render guards in one place — no band, no desktop sidebar, gated
// console in its new skypane home.
func TestCalendarV2Page_NoBand_NoDesktopSidebar_ConsoleInSkypane(t *testing.T) {
	html := renderSkypanePage(t, skypaneDetachPageData(true))

	// 1) NO always-on worldState band (and no band region wrapper) on the page.
	for _, forbidden := range []string{
		"cal-v2-worldstate-band",   // the 200px sky band section
		"data-cal-v2-worldstate-band",
		"data-cal-band-region",     // the relative overlay region the band + console shared
	} {
		if strings.Contains(html, forbidden) {
			t.Errorf("detached page must not render the worldState band; found %q", forbidden)
		}
	}

	// 2) NO desktop mini-month sidebar: the sidebar aside is still emitted (it is
	// the mobile navigator for non-Month views) but must never be desktop-visible
	// — for this Month payload it is fully hidden, and `md:block` must be gone.
	if strings.Contains(html, "md:block") && strings.Contains(html, "data-cal-v2-sidebar") {
		// Only fail if the sidebar itself carries md:block; a stray md:block
		// elsewhere on the shell is not our concern. Scope to the aside.
		if i := strings.Index(html, `data-cal-v2-sidebar`); i >= 0 {
			start := strings.LastIndex(html[:i], "<aside")
			end := strings.Index(html[i:], "</aside>")
			if start >= 0 && end >= 0 && strings.Contains(html[start:i+end], "md:block") {
				t.Error("desktop mini-month sidebar must be detached (the aside must not carry md:block)")
			}
		}
	}

	// 3) The GM world-state console is re-homed INSIDE the skypane wrapper, and
	// the sky strip is present — the console before the strip (its tab rides the
	// strip's top edge).
	sky := strings.Index(html, "data-cal-skypane")
	if sky < 0 {
		t.Fatal("skypane wrapper (data-cal-skypane) missing")
	}
	panel := strings.Index(html, "data-gm-panel")
	strip := strings.Index(html, "data-cal-sky-strip")
	if panel < 0 {
		t.Error("capability holder must get the GM console (data-gm-panel) in its new home")
	}
	if strip < 0 {
		t.Error("sky strip (data-cal-sky-strip) must render on the page")
	}
	if panel >= 0 && strip >= 0 && !(sky < panel && panel < strip) {
		t.Errorf("GM console must sit inside the skypane, before the sky strip; got skypane=%d panel=%d strip=%d", sky, panel, strip)
	}

	// 4) The skybox engine seed + assets the band used to load now ride the sky
	// strip, so the strip's skybox + the console's live re-render keep working.
	for _, want := range []string{
		`id="cal-v2-worldstate"`,             // the relocated seed blob
		"/static/js/cal-almanac.js",          // the relocated shared engine
		"/static/js/widgets/skybox.js",       // the relocated skybox widget
		"/static/css/cal-almanac-render.css", // the relocated render CSS
	} {
		if !strings.Contains(html, want) {
			t.Errorf("sky strip must carry the relocated engine asset %q", want)
		}
	}
}

// TestCalendarV2Page_PlayerGetsNoConsole: a player (no world-state control)
// gets the sky strip but never the GM console markup — the gate stays
// server-side, and the band is gone for players too.
func TestCalendarV2Page_PlayerGetsNoConsole(t *testing.T) {
	html := renderSkypanePage(t, skypaneDetachPageData(false))
	if strings.Contains(html, "data-gm-panel") {
		t.Error("players must not receive the GM console markup")
	}
	if !strings.Contains(html, "data-cal-sky-strip") {
		t.Error("players still get the sky strip")
	}
	if strings.Contains(html, "cal-v2-worldstate-band") {
		t.Error("the worldState band must be gone for players too")
	}
}
