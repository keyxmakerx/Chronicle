// calendar_v2_mobile_agenda_test.go — C-CAL-MOBILE-AGENDA. Covers the pure
// helpers (mini-month dots, agenda grouping, moon glyphs) plus render/source
// pins for the breakpoint swap, mirroring the #542 design-pass-1 test style
// (calendar_v2_design_pass1_test.go) since screenshot checks aren't available
// in this harness.
package calendar

import (
	"context"
	"strings"
	"testing"
)

// mobileAgendaCalendar builds a small fantasy calendar with a moon + two
// event categories, reused across this file's tests.
func mobileAgendaCalendar() *Calendar {
	return &Calendar{
		ID:   "cal-1",
		Name: "The Sunless Reaches",
		Months: []Month{
			{Name: "Harvestwane", Days: 30},
			{Name: "Frostmere", Days: 30},
		},
		Weekdays: []Weekday{
			{Name: "Mon"}, {Name: "Tue"}, {Name: "Wed"}, {Name: "Thu"},
			{Name: "Fri"}, {Name: "Sat"}, {Name: "Sun"},
		},
		Moons:        []Moon{{Name: "Selûne", CycleDays: 32, PhaseOffset: 0}},
		CurrentYear:  1492,
		CurrentMonth: 1,
		CurrentDay:   13,
		EventCategories: []EventCategory{
			{Slug: "social", Name: "Session", Color: "#2a78d6"},
			{Slug: "festival", Name: "Festival", Color: "#eda100"},
		},
	}
}

// --- Mini-month dots -----------------------------------------------------

func TestMobileMiniMonthDays_NilCalendarReturnsNil(t *testing.T) {
	if got := mobileMiniMonthDays(CalendarV2ViewData{}); got != nil {
		t.Errorf("nil calendar → nil days; got %+v", got)
	}
}

func TestMobileMiniMonthDays_SameOffsetAsDesktopSidebar(t *testing.T) {
	cal := mobileAgendaCalendar()
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 1492, Month: 1, Day: 13}
	desktop := miniMonthDays(data)
	mobile := mobileMiniMonthDays(data)
	if len(desktop) != len(mobile) {
		t.Fatalf("mobile mini-month must have the same cell count as the desktop sidebar (same offset); desktop=%d mobile=%d", len(desktop), len(mobile))
	}
	for i := range desktop {
		if desktop[i].Day != mobile[i].Day {
			t.Fatalf("cell %d: desktop day=%d mobile day=%d — offsets diverged", i, desktop[i].Day, mobile[i].Day)
		}
	}
}

func TestMobileMiniMonthDays_TodayAndSelected(t *testing.T) {
	cal := mobileAgendaCalendar()
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 1492, Month: 1, Day: 20}
	days := mobileMiniMonthDays(data)
	var day13, day20 *mobileMiniMonthDay
	for i := range days {
		if days[i].Day == 13 {
			day13 = &days[i]
		}
		if days[i].Day == 20 {
			day20 = &days[i]
		}
	}
	if day13 == nil || !day13.IsToday || day13.IsSelected {
		t.Errorf("day 13: want IsToday=true IsSelected=false; got %+v", day13)
	}
	if day20 == nil || day20.IsToday || !day20.IsSelected {
		t.Errorf("day 20: want IsToday=false IsSelected=true; got %+v", day20)
	}
}

func TestMobileMiniMonthDotColors_MatchesGridPipsAndCaps(t *testing.T) {
	cal := mobileAgendaCalendar()
	events := []Event{
		timedEvent("a", "One", 16, 9, 0),
		timedEvent("b", "Two", 16, 10, 0),
		timedEvent("c", "Three", 16, 11, 0),
		timedEvent("d", "Four", 16, 12, 0), // 4th event — should be capped out
	}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 1492, Month: 1, Day: 1, Events: events}
	dots := mobileMiniMonthDotColors(data, 16)
	if len(dots) != mobileMiniMonthDotsCap() {
		t.Fatalf("expected %d dots (capped); got %d", mobileMiniMonthDotsCap(), len(dots))
	}
	for _, c := range dots {
		if c != "#2a78d6" {
			t.Errorf("expected the social category color #2a78d6; got %q", c)
		}
	}
	if got := mobileMiniMonthDotColors(data, 5); got != nil {
		t.Errorf("day with no events → nil dots; got %+v", got)
	}
}

func TestMobileMiniMonthDotColors_ExcludesMultiDayEvents(t *testing.T) {
	cal := mobileAgendaCalendar()
	end := 18
	ev := timedEvent("multi", "Long trek", 16, 9, 0)
	ev.EndDay = &end
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 1492, Month: 1, Day: 1, Events: []Event{ev}}
	if got := mobileMiniMonthDotColors(data, 16); got != nil {
		t.Errorf("a multi-day event's START day should not get a dot (ribbon layer handles it); got %+v", got)
	}
}

func TestMobileMiniMonthDayClasses(t *testing.T) {
	sel := mobileMiniMonthDayClasses(mobileMiniMonthDay{Day: 5, IsSelected: true})
	if !strings.Contains(sel, "bg-accent") || !strings.Contains(sel, "text-white") {
		t.Errorf("selected day should be a filled accent pill; got %q", sel)
	}
	today := mobileMiniMonthDayClasses(mobileMiniMonthDay{Day: 5, IsToday: true})
	if !strings.Contains(today, "ring-accent") || strings.Contains(today, "bg-accent") {
		t.Errorf("today (unselected) should be a ring only, not a fill; got %q", today)
	}
	plain := mobileMiniMonthDayClasses(mobileMiniMonthDay{Day: 5})
	if strings.Contains(plain, "bg-accent") || strings.Contains(plain, "ring-accent") {
		t.Errorf("plain day should not accent; got %q", plain)
	}
}

func TestMobileMiniMonthDotColor_WhiteWhenSelected(t *testing.T) {
	if got := mobileMiniMonthDotColor("#2a78d6", true); got != "#fff" {
		t.Errorf("selected cell's dots must go white (contrast against the filled pill); got %q", got)
	}
	if got := mobileMiniMonthDotColor("#2a78d6", false); got != "#2a78d6" {
		t.Errorf("unselected cell's dots keep the category color; got %q", got)
	}
}

// --- Sidebar hide (avoids a duplicate mini-month on mobile) ----------------

func TestMobileSidebarClasses_HiddenOnlyForMonthView(t *testing.T) {
	month := mobileSidebarClasses(CalendarV2ViewData{View: "month"})
	if !strings.Contains(month, "hidden") || !strings.Contains(month, "md:block") {
		t.Errorf("month view: sidebar must hide at <768px (mobileMonthAssembly replaces it); got %q", month)
	}
	for _, v := range []string{"week", "day", "ledger"} {
		got := mobileSidebarClasses(CalendarV2ViewData{View: v})
		if strings.Contains(got, "hidden") {
			t.Errorf("view %q: sidebar must keep its current (unchanged) mobile presentation; got %q", v, got)
		}
	}
	// Base classes are preserved regardless of the hide branch.
	if !strings.Contains(month, "w-60") {
		t.Errorf("hidden branch must still carry the base sidebar classes; got %q", month)
	}
}

// --- Agenda grouping --------------------------------------------------------

func TestMobileAgendaGroups_StartsFromSelectedDayAndSkipsEmpty(t *testing.T) {
	cal := mobileAgendaCalendar()
	events := []Event{
		timedEvent("early", "Before cursor", 10, 9, 0), // day 10 — before the day=16 cursor
		allDayEvent("fair", "Merchant fair", 16),
		timedEvent("session", "Session 14", 16, 19, 0),
		allDayEvent("craft", "Crafting day", 18),
	}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 1492, Month: 1, Day: 16, Events: events}
	groups := mobileAgendaGroups(data)
	if len(groups) != 2 {
		t.Fatalf("expected 2 day-groups (16, 18); got %d: %+v", len(groups), groups)
	}
	if groups[0].Day != 16 || len(groups[0].Events) != 2 {
		t.Errorf("first group should be day 16 with 2 events; got day=%d events=%d", groups[0].Day, len(groups[0].Events))
	}
	if groups[1].Day != 18 || len(groups[1].Events) != 1 {
		t.Errorf("second group should be day 18 with 1 event; got day=%d events=%d", groups[1].Day, len(groups[1].Events))
	}
	// Day 10's event is before the cursor — must not appear anywhere.
	for _, g := range groups {
		for _, e := range g.Events {
			if e.EventID == "early" {
				t.Errorf("event before the selected day must not appear in the agenda")
			}
		}
	}
}

func TestMobileAgendaGroups_StopsAtMonthEnd(t *testing.T) {
	cal := mobileAgendaCalendar() // month 1 has 30 days
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 1492, Month: 1, Day: 29, Events: []Event{
		allDayEvent("a", "Late event", 30),
	}}
	groups := mobileAgendaGroups(data)
	if len(groups) != 1 || groups[0].Day != 30 {
		t.Fatalf("expected exactly the day-30 group; got %+v", groups)
	}
}

func TestMobileAgendaGroups_NoEventsReturnsEmpty(t *testing.T) {
	cal := mobileAgendaCalendar()
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 1492, Month: 1, Day: 16}
	if got := mobileAgendaGroups(data); len(got) != 0 {
		t.Errorf("no events → no groups; got %+v", got)
	}
}

func TestMobileAgendaGroups_ExcludesMultiDayEvents(t *testing.T) {
	cal := mobileAgendaCalendar()
	end := 20
	ev := timedEvent("multi", "Long trek", 16, 9, 0)
	ev.EndDay = &end
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 1492, Month: 1, Day: 16, Events: []Event{ev}}
	if got := mobileAgendaGroups(data); len(got) != 0 {
		t.Errorf("a multi-day event must not create an agenda day-group (ribbon layer owns it, mirrors the day popover's own filter); got %+v", got)
	}
}

func TestMobileAgendaGroups_NilCalendarReturnsNil(t *testing.T) {
	if got := mobileAgendaGroups(CalendarV2ViewData{}); got != nil {
		t.Errorf("nil calendar → nil groups; got %+v", got)
	}
}

func TestMobileAgendaDayHeader_WeekdayDayMonth(t *testing.T) {
	cal := mobileAgendaCalendar()
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 1492, Month: 1, Day: 1}
	got := mobileAgendaDayHeader(data, 16)
	if !strings.Contains(got, "16") || !strings.Contains(got, "Harvestwane") {
		t.Errorf("header should carry the day number + month name; got %q", got)
	}
}

// --- Moon-phase glyph -------------------------------------------------------

func TestMoonPhaseGlyph_AllEightNamesMapped(t *testing.T) {
	names := []string{
		"New Moon", "Waxing Crescent", "First Quarter", "Waxing Gibbous",
		"Full Moon", "Waning Gibbous", "Last Quarter", "Waning Crescent",
	}
	seen := map[string]bool{}
	for _, n := range names {
		g := moonPhaseGlyph(n)
		if g == "" {
			t.Errorf("phase %q must map to a glyph", n)
		}
		if seen[g] {
			t.Errorf("glyph %q reused for phase %q — each phase should read distinctly", g, n)
		}
		seen[g] = true
	}
	if got := moonPhaseGlyph("nonsense"); got != "" {
		t.Errorf("unknown phase name should map to empty (fail closed, no misleading glyph); got %q", got)
	}
}

func TestMobileAgendaMoonGlyph_EmptyWhenNoMoons(t *testing.T) {
	cal := mobileAgendaCalendar()
	cal.Moons = nil
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 1492, Month: 1}
	if got := mobileAgendaMoonGlyph(data, 16); got != "" {
		t.Errorf("no moons on the calendar → no glyph; got %q", got)
	}
}

func TestMobileAgendaMoonGlyph_NilCalendar(t *testing.T) {
	if got := mobileAgendaMoonGlyph(CalendarV2ViewData{}, 16); got != "" {
		t.Errorf("nil calendar → no glyph; got %q", got)
	}
}

// --- Agenda card projection --------------------------------------------------

func TestMobileAgendaEventsForDay_AllDayFallsBackToLabel(t *testing.T) {
	cal := mobileAgendaCalendar()
	data := CalendarV2ViewData{ActiveCalendar: cal}
	out := mobileAgendaEventsForDay(data, []Event{allDayEvent("a", "Merchant fair", 16)})
	if len(out) != 1 || out[0].TimeLabel != "All day" {
		t.Errorf("all-day event should get the 'All day' fallback label; got %+v", out)
	}
	if out[0].Category != "Festival" {
		t.Errorf("expected category name 'Festival'; got %q", out[0].Category)
	}
}

func TestMobileAgendaEventsForDay_TimedKeepsFullRange(t *testing.T) {
	cal := mobileAgendaCalendar()
	data := CalendarV2ViewData{ActiveCalendar: cal}
	e := timedEvent("s", "Session 14", 16, 19, 0)
	eh, em := 23, 0
	e.EndHour, e.EndMinute = &eh, &em
	out := mobileAgendaEventsForDay(data, []Event{e})
	if len(out) != 1 || out[0].TimeLabel != "19:00 — 23:00" {
		t.Errorf("agenda card should keep the FULL time range (unlike the cramped grid pip, which truncates to start-only); got %+v", out)
	}
}

func TestMobileAgendaCardColor_FallsBackToMutedToken(t *testing.T) {
	if got := mobileAgendaCardColor(""); got == "" {
		t.Error("an event with no category color should still get a reserved (non-empty) accent slot")
	}
	if got := mobileAgendaCardColor("#2a78d6"); got != "#2a78d6" {
		t.Errorf("a colored event should keep its own color; got %q", got)
	}
}

// --- Render / source pins (mirrors #542's design-pass-1 style — no
// screenshot harness available, so DOM/source markers stand in) -----------

// TestMonthViewPlaceholder_BothBreakpointBranchesRenderPresentButHidden: the
// dispatch's §5 requirement — the desktop grid and the mobile assembly BOTH
// render (present-but-hidden), the breakpoint is pure CSS, not an HTMX
// fragment swap.
func TestMonthViewPlaceholder_BothBreakpointBranchesRenderPresentButHidden(t *testing.T) {
	cal := mobileAgendaCalendar()
	data := CalendarV2ViewData{ActiveCalendar: cal, View: "month", Year: 1492, Month: 1, Day: 16, CampaignID: "camp-1"}
	var sb strings.Builder
	if err := monthViewPlaceholder(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, `data-month-desktop-grid="true"`) || !strings.Contains(html, "hidden md:block") {
		t.Error("desktop grid must render present-but-hidden at <768px (hidden md:block), not be omitted")
	}
	if !strings.Contains(html, `data-mobile-month-assembly="true"`) || !strings.Contains(html, "md:hidden") {
		t.Error("mobile assembly must render present-but-hidden at >=768px (md:hidden), not be omitted")
	}
}

// TestMobileMiniMonth_NumberFixedCellDotsReservedAbsoluteLayer: the operator's
// alignment rule applied verbatim to the mobile mini-month — the day number
// sits in a fixed h-10 grid cell; the dots layer is `absolute` (out of flow),
// so it can never move the number regardless of how many dots render.
func TestMobileMiniMonth_NumberFixedCellDotsReservedAbsoluteLayer(t *testing.T) {
	cal := mobileAgendaCalendar()
	events := []Event{timedEvent("a", "One", 16, 9, 0)}
	data := CalendarV2ViewData{ActiveCalendar: cal, View: "month", Year: 1492, Month: 1, Day: 16, CampaignID: "camp-1", Events: events}
	var sb strings.Builder
	if err := mobileMiniMonth(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "h-10") {
		t.Error("every mini-month day cell must sit in a fixed h-10 (40px) box")
	}
	if !strings.Contains(html, "absolute bottom-1 left-1/2 -translate-x-1/2") {
		t.Error("dots must render in a reserved, absolutely-positioned layer (removed from flow)")
	}
	// A cell WITHOUT events must not render the dots wrapper span at all
	// (day 1 has no events in this fixture) — no empty reserved layer noise.
	day1 := html[:strings.Index(html, ">2<")]
	if strings.Contains(day1, `aria-hidden="true">`) && strings.Contains(day1, "dots") {
		t.Error("an event-free day should not render an empty dots wrapper")
	}
}

// TestMobileAgendaCard_CarriesExistingEventCardHooks: the dispatch's "tap
// card → the existing quick-edit/detail flow" — the SAME [data-event-card]
// click wiring event_grid.js already binds document-wide must be able to
// find this button (no new JS needed for the tap-to-open interaction).
func TestMobileAgendaCard_CarriesExistingEventCardHooks(t *testing.T) {
	ev := mobileAgendaEvent{EventID: "evt-9", Title: "Fence meeting", TimeLabel: "21:30", Color: "#e34948", IsPublic: true}
	var sb strings.Builder
	if err := mobileAgendaCard(ev).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	for _, want := range []string{
		`data-event-card="agenda"`,
		`data-event-id="evt-9"`,
		"<button",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("agenda card missing %q\n%s", want, html)
		}
	}
	// Actions (the chevron affordance) must be visible AT REST — no
	// hover-only/opacity-0 gating (pointer:coarse requirement, §3).
	if strings.Contains(html, "opacity-0") || strings.Contains(html, "group-hover") {
		t.Error("agenda card's affordance must be visible at rest, not hover-gated")
	}
}

func TestMobileAgendaCard_LockIconForNonPublic(t *testing.T) {
	pub := mobileAgendaEvent{EventID: "a", Title: "T", TimeLabel: "All day", IsPublic: true}
	priv := mobileAgendaEvent{EventID: "b", Title: "T", TimeLabel: "All day", IsPublic: false}
	var sbPub, sbPriv strings.Builder
	_ = mobileAgendaCard(pub).Render(context.Background(), &sbPub)
	_ = mobileAgendaCard(priv).Render(context.Background(), &sbPriv)
	if strings.Contains(sbPub.String(), "fa-lock") {
		t.Error("a public event should not show the lock glyph")
	}
	if !strings.Contains(sbPriv.String(), "fa-lock") {
		t.Error("a non-public event should show the lock glyph")
	}
}

// TestCalendarV2ViewSwitcher_MobileReduction: phone command bar reduces to
// Month/Agenda — Week/Day/Timeline hidden at <768px (§4).
func TestCalendarV2ViewSwitcher_MobileReduction(t *testing.T) {
	cal := mobileAgendaCalendar()
	data := CalendarV2ViewData{ActiveCalendar: cal, AllCalendars: []Calendar{*cal}, View: "month", CampaignID: "camp-1", Year: 1492, Month: 1, Day: 13}
	var sb strings.Builder
	if err := calendarV2ViewSwitcher(nil, data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, ">Agenda<") {
		t.Error("switcher missing the mobile Agenda pill")
	}
	if !strings.Contains(html, `class="md:hidden`) {
		t.Error("Agenda pill must be phone-only (md:hidden)")
	}
	if !strings.Contains(html, `<span class="hidden md:contents">`) {
		t.Error("Week/Day/Timeline must be wrapped in a hidden md:contents group so they vanish on phones without affecting desktop's pill layout")
	}
	// Agenda routes to the SAME href as the Month pill (no separate server
	// view exists — "no new endpoints"). templ HTML-escapes the query
	// string's "&" to "&amp;" on render, so mirror that before comparing.
	monthHref := strings.ReplaceAll(string(v2ViewHref(data, "month")), "&", "&amp;")
	if !strings.Contains(html, `href="`+monthHref+`"`) {
		t.Errorf("Agenda pill must link to the same month-view route the Month pill uses; want href containing %q in:\n%s", monthHref, html)
	}
}

// TestMiniMonthV2Sidebar_HiddenOnMobileOnlyForMonthView: avoids a duplicate
// mini-month rendering alongside mobileMonthAssembly.
func TestMiniMonthV2Sidebar_HiddenOnMobileOnlyForMonthView(t *testing.T) {
	cal := mobileAgendaCalendar()
	monthData := CalendarV2ViewData{ActiveCalendar: cal, View: "month", CampaignID: "camp-1", Year: 1492, Month: 1, Day: 13}
	var sbMonth strings.Builder
	if err := miniMonthV2Sidebar(monthData).Render(context.Background(), &sbMonth); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(sbMonth.String(), "hidden md:block") {
		t.Error("month view: sidebar must hide at <768px")
	}
	weekData := monthData
	weekData.View = "week"
	var sbWeek strings.Builder
	if err := miniMonthV2Sidebar(weekData).Render(context.Background(), &sbWeek); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(sbWeek.String(), "hidden md:block") {
		t.Error("week view: sidebar must keep its current (unchanged) presentation")
	}
}

// TestSource_MonthViewPlaceholderIsCSSSwapNotFragment: guards the dispatch
// §1 design call (CSS breakpoint swap, not an HTMX fragment swap) against
// silent drift — both branches must live in the SAME templ render path.
func TestSource_MonthViewPlaceholderIsCSSSwapNotFragment(t *testing.T) {
	src := readRepoFile(t, "internal/plugins/calendar/calendar_v2.templ")
	fn := src[strings.Index(src, "templ monthViewPlaceholder"):]
	fn = fn[:strings.Index(fn, "\ntempl ")]
	if !strings.Contains(fn, "hidden md:block") || !strings.Contains(fn, "mobileMonthAssembly") {
		t.Error("monthViewPlaceholder must render both the desktop grid (hidden md:block) and mobileMonthAssembly in one pass")
	}
}
