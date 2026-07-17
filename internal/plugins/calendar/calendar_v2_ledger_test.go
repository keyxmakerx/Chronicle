// calendar_v2_ledger_test.go — the Timeline Ledger, the calendar's 4th V2 view
// (C-CAL-TIMELINE-V2-W1). Covers the pure assembly (ledgerRows / ledgerEraGroups
// / labels), the year-step nav (v2Step "ledger"), the 4th view pill + active
// state, and the server-rendered row markup (era band, epoch year, span bar,
// dm_only lock, drawer-open data attributes).
//
// The dm_only "absent for a player" contract is enforced upstream by the
// service's filterEventsByUser (tested there): the Ledger renders exactly the
// events it is handed. These tests pin BOTH sides of that contract at the render
// layer — a dm_only event in the slice draws the lock (the GM's view); the same
// slice with it filtered out draws no lock (the post-filter player view).

package calendar

import (
	"context"
	"strings"
	"testing"
)

func ledgerIntPtr(n int) *int       { return &n }
func ledgerStrPtr(s string) *string { return &s }

// ledgerTestCalendar is a 3-month Harptos-like calendar with two eras and an
// epoch, enough to exercise era grouping + epoch-suffixed year labels.
func ledgerTestCalendar() *Calendar {
	return &Calendar{
		ID:          "cal-1",
		Name:        "Harptos",
		EpochName:   ledgerStrPtr("DR"),
		CurrentYear: 1492, CurrentMonth: 1, CurrentDay: 1,
		Months: []Month{
			{Name: "Hammer", Days: 30},
			{Name: "Alturiak", Days: 30},
			{Name: "Ches", Days: 30},
		},
		Weekdays: []Weekday{
			{Name: "Sul"}, {Name: "Mol"}, {Name: "Zor"}, {Name: "Ere"},
			{Name: "Ver"}, {Name: "Sar"}, {Name: "Tar"},
		},
		EventCategories: []EventCategory{
			{Slug: "war", Name: "War", Color: "#cc3333"},
			{Slug: "politics", Name: "Politics", Color: "#33aa55"},
		},
		Eras: []Era{
			{Name: "Age of Conflict", StartYear: 1488, EndYear: ledgerIntPtr(1490), Color: "#aa3333"},
			{Name: "The Mythic Restoration", StartYear: 1491, EndYear: nil, Color: "#8833aa"},
		},
	}
}

func ledgerTestData(year int, events []Event) CalendarV2ViewData {
	return CalendarV2ViewData{
		ActiveCalendar: ledgerTestCalendar(),
		View:           "ledger",
		Year:           year,
		Month:          1,
		Day:            1,
		CampaignID:     "camp-1",
		Events:         events,
	}
}

func renderLedger(t *testing.T, data CalendarV2ViewData) string {
	t.Helper()
	var sb strings.Builder
	if err := ledgerView(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render ledger: %v", err)
	}
	return sb.String()
}

// --- Assembly ---------------------------------------------------------------

// TestLedgerRows_SingleDayChronologicalOrder: single-day events emit one row
// each, ordered by date (month → day iteration).
func TestLedgerRows_SingleDayChronologicalOrder(t *testing.T) {
	events := []Event{
		{ID: "c", Name: "Third", Year: 1492, Month: 3, Day: 10, Visibility: "everyone"},
		{ID: "a", Name: "First", Year: 1492, Month: 1, Day: 5, Visibility: "everyone"},
		{ID: "b", Name: "Second", Year: 1492, Month: 2, Day: 1, Visibility: "everyone"},
	}
	rows := ledgerRows(ledgerTestData(1492, events))
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows; got %d", len(rows))
	}
	wantOrder := []string{"a", "b", "c"}
	for i, w := range wantOrder {
		if rows[i].EventID != w {
			t.Errorf("row %d: expected %q, got %q (order not chronological)", i, w, rows[i].EventID)
		}
	}
	if rows[0].DateLabel != "Hammer 5" {
		t.Errorf("expected first row date 'Hammer 5'; got %q", rows[0].DateLabel)
	}
}

// TestLedgerRows_MultiDayEmitsOneSpannedRow: a multi-day event yields exactly
// one row (anchored on its start day) with Span=true and a range date label.
func TestLedgerRows_MultiDayEmitsOneSpannedRow(t *testing.T) {
	events := []Event{
		{
			ID: "council", Name: "Council of the Marches",
			Year: 1492, Month: 3, Day: 12,
			EndYear: ledgerIntPtr(1492), EndMonth: ledgerIntPtr(3), EndDay: ledgerIntPtr(19),
			Visibility: "everyone",
		},
	}
	rows := ledgerRows(ledgerTestData(1492, events))
	if len(rows) != 1 {
		t.Fatalf("multi-day event should emit exactly one row; got %d", len(rows))
	}
	if !rows[0].Span {
		t.Error("multi-day row should have Span=true")
	}
	if rows[0].DateLabel != "Ches 12 – 19" {
		t.Errorf("expected same-month span label 'Ches 12 – 19'; got %q", rows[0].DateLabel)
	}
}

// TestLedgerRows_MultiDayCrossMonthLabel: a span crossing months shows both
// month names in the date range.
func TestLedgerRows_MultiDayCrossMonthLabel(t *testing.T) {
	events := []Event{
		{
			ID: "march", Name: "The Long March",
			Year: 1492, Month: 1, Day: 28,
			EndYear: ledgerIntPtr(1492), EndMonth: ledgerIntPtr(2), EndDay: ledgerIntPtr(3),
			Visibility: "everyone",
		},
	}
	rows := ledgerRows(ledgerTestData(1492, events))
	if len(rows) != 1 {
		t.Fatalf("expected 1 row; got %d", len(rows))
	}
	if rows[0].DateLabel != "Hammer 28 – Alturiak 3" {
		t.Errorf("expected cross-month span label; got %q", rows[0].DateLabel)
	}
}

// TestLedgerRows_RecurringExpandsAcrossYear: a monthly-recurring event expands
// into one row per month in the displayed year (the projection the month grid
// would show, flattened into the chronicle).
func TestLedgerRows_RecurringExpandsAcrossYear(t *testing.T) {
	events := []Event{
		{
			ID: "tithe", Name: "Monthly Tithe",
			Year: 1492, Month: 1, Day: 5,
			IsRecurring: true, RecurrenceType: ledgerStrPtr(RecurrenceMonthly),
			Visibility: "everyone",
		},
	}
	rows := ledgerRows(ledgerTestData(1492, events))
	if len(rows) != 3 {
		t.Fatalf("monthly recurrence over a 3-month year should emit 3 rows; got %d", len(rows))
	}
	// Each occurrence carries its own month's date label (not the base date).
	wantDates := []string{"Hammer 5", "Alturiak 5", "Ches 5"}
	for i, w := range wantDates {
		if rows[i].DateLabel != w {
			t.Errorf("recurrence row %d: expected date %q; got %q", i, w, rows[i].DateLabel)
		}
	}
}

// TestLedgerRows_MultiDaySpillFromPriorYear: a span that started in a prior
// year but reaches into the displayed year surfaces anchored on the year's
// first day. NOTE: this exercises ledgerRows directly — with W1's data layer
// the repo never returns non-recurring prior-year rows, so this branch is
// defensive until W2's multi-year query (see ledgerMultiDayAnchor's comment).
func TestLedgerRows_MultiDaySpillFromPriorYear(t *testing.T) {
	events := []Event{
		{
			ID: "siege", Name: "The Winter Siege",
			Year: 1491, Month: 3, Day: 25,
			EndYear: ledgerIntPtr(1492), EndMonth: ledgerIntPtr(1), EndDay: ledgerIntPtr(10),
			Visibility: "everyone",
		},
	}
	rows := ledgerRows(ledgerTestData(1492, events))
	if len(rows) != 1 {
		t.Fatalf("prior-year span reaching into the year should emit 1 row; got %d", len(rows))
	}
	if !rows[0].Span {
		t.Error("spill row should keep Span=true")
	}
}

// TestLedgerEraGroups_SelectsEraForDisplayedYear: the group carries the era
// containing the displayed year, with an epoch-suffixed year label.
func TestLedgerEraGroups_SelectsEraForDisplayedYear(t *testing.T) {
	groups := ledgerEraGroups(ledgerTestData(1492, nil))
	if len(groups) != 1 {
		t.Fatalf("W1 emits exactly one era group; got %d", len(groups))
	}
	if groups[0].Name != "The Mythic Restoration" {
		t.Errorf("expected 1492's era; got %q", groups[0].Name)
	}
	if groups[0].Span != "1491 DR – ongoing" {
		t.Errorf("expected ongoing era span; got %q", groups[0].Span)
	}
	if len(groups[0].Years) != 1 || groups[0].Years[0].Label != "1492 DR" {
		t.Errorf("expected single epoch-suffixed year label '1492 DR'; got %+v", groups[0].Years)
	}
}

// TestLedgerEraGroups_ClosedEraSpan: a bounded era renders a start–end span.
func TestLedgerEraGroups_ClosedEraSpan(t *testing.T) {
	groups := ledgerEraGroups(ledgerTestData(1489, nil))
	if groups[0].Name != "Age of Conflict" {
		t.Fatalf("expected 1489's era; got %q", groups[0].Name)
	}
	if groups[0].Span != "1488 – 1490 DR" {
		t.Errorf("expected closed era span '1488 – 1490 DR'; got %q", groups[0].Span)
	}
}

// --- Year stepping ----------------------------------------------------------

// TestV2Step_TimelineStepsByYear: the Ledger's prev/next step by whole years,
// preserving the month/day cursor so switching back to Month/Week/Day lands
// where you were.
func TestV2Step_TimelineStepsByYear(t *testing.T) {
	data := CalendarV2ViewData{ActiveCalendar: ledgerTestCalendar(), View: "ledger", Year: 1492, Month: 3, Day: 15}
	if y, m, d := v2Step(data, 1); y != 1493 || m != 3 || d != 15 {
		t.Errorf("next-year step = (%d,%d,%d); want (1493,3,15)", y, m, d)
	}
	if y, m, d := v2Step(data, -1); y != 1491 || m != 3 || d != 15 {
		t.Errorf("prev-year step = (%d,%d,%d); want (1491,3,15)", y, m, d)
	}
}

// --- View switcher (4th pill) ----------------------------------------------

// TestViewSwitcher_TimelinePillActive: on the Timeline view the 4th pill is
// the selected tab; the other three link out. All four pills render.
func TestViewSwitcher_TimelinePillActive(t *testing.T) {
	var sb strings.Builder
	data := CalendarV2ViewData{ActiveCalendar: ledgerTestCalendar(), View: "ledger", CampaignID: "camp-1", Year: 1492, Month: 1, Day: 1}
	if err := calendarV2ViewSwitcher(nil, data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render switcher: %v", err)
	}
	html := sb.String()
	for _, label := range []string{"Month", "Week", "Day", "Timeline"} {
		if !strings.Contains(html, ">"+label+"<") {
			t.Errorf("switcher missing %q pill", label)
		}
	}
	// The active Timeline pill is a <span aria-selected="true">, not a link. The
	// route segment is "ledger" (avoids the timeline-plugin slug collision).
	if !strings.Contains(html, `aria-selected="true"`) {
		t.Error("expected an active (aria-selected=true) pill")
	}
	if strings.Contains(html, `/ledger?`) {
		t.Error("active Timeline pill should not be a link to /ledger")
	}
}

// TestViewSwitcher_TimelinePillInactiveLinks: from Month view the Timeline pill
// is a link to the ledger route.
func TestViewSwitcher_TimelinePillInactiveLinks(t *testing.T) {
	var sb strings.Builder
	data := CalendarV2ViewData{ActiveCalendar: ledgerTestCalendar(), View: "month", CampaignID: "camp-1", Year: 1492, Month: 1, Day: 1}
	if err := calendarV2ViewSwitcher(nil, data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render switcher: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "/ledger?") {
		t.Error("inactive Timeline pill should link to the /ledger route")
	}
}

// --- Rendered Ledger --------------------------------------------------------

// TestLedgerView_RendersEraYearAndRow: the rendered chronicle carries the era
// header, epoch year label, a clickable event row (drawer data attributes), and
// the category chip.
func TestLedgerView_RendersEraYearAndRow(t *testing.T) {
	warSlug := "war"
	events := []Event{
		{ID: "evt-siege", Name: "The Siege of Highmoon", Year: 1492, Month: 1, Day: 3, Category: &warSlug, Visibility: "everyone", EntityName: "Highmoon"},
	}
	html := renderLedger(t, ledgerTestData(1492, events))
	for _, want := range []string{
		"The Mythic Restoration",    // era header
		"1491 DR – ongoing",         // era span
		"1492 DR",                   // epoch year label
		"The Siege of Highmoon",     // event title
		`data-event-card="ledger"`,  // opens existing drawer…
		`data-event-id="evt-siege"`, // …by event id
		"Hammer 3",                  // computed date column
		"@Highmoon",                 // entity annotation
		"War",                       // category chip label
	} {
		if !strings.Contains(html, want) {
			t.Errorf("ledger render missing %q", want)
		}
	}
}

// TestLedgerView_MultiDayDrawsSpanBar: a multi-day event draws the span bar.
func TestLedgerView_MultiDayDrawsSpanBar(t *testing.T) {
	events := []Event{
		{
			ID: "council", Name: "Council of the Marches",
			Year: 1492, Month: 3, Day: 12,
			EndYear: ledgerIntPtr(1492), EndMonth: ledgerIntPtr(3), EndDay: ledgerIntPtr(19),
			Visibility: "everyone",
		},
	}
	html := renderLedger(t, ledgerTestData(1492, events))
	if !strings.Contains(html, "bg-accent/70") {
		t.Error("multi-day event should render the span bar (bg-accent/70)")
	}
	if !strings.Contains(html, "Ches 12 – 19") {
		t.Errorf("expected span date label in render; got:\n%s", html)
	}
}

// TestLedgerView_DMOnlyLockGMvsPlayer: a dm_only event present in the slice (the
// GM's post-filter view) draws the lock; the same slice with it filtered out
// (the player's post-filter view) draws no lock. filterEventsByUser owns the
// actual removal — the Ledger renders what it's handed.
func TestLedgerView_DMOnlyLockGMvsPlayer(t *testing.T) {
	secret := Event{ID: "secret", Name: "The Hidden Pact", Year: 1492, Month: 1, Day: 8, Visibility: "dm_only"}
	public := Event{ID: "pub", Name: "The Public Festival", Year: 1492, Month: 1, Day: 8, Visibility: "everyone"}

	// GM view: both events present → lock renders + secret title shows.
	gm := renderLedger(t, ledgerTestData(1492, []Event{public, secret}))
	if !strings.Contains(gm, "fa-lock") {
		t.Error("GM view: dm_only event should render the lock")
	}
	if !strings.Contains(gm, "The Hidden Pact") {
		t.Error("GM view: dm_only event title should render")
	}

	// Player view: service already stripped the dm_only event → no lock, no title.
	player := renderLedger(t, ledgerTestData(1492, []Event{public}))
	if strings.Contains(player, "fa-lock") {
		t.Error("player view: no dm_only rows, so no lock should render")
	}
	if strings.Contains(player, "The Hidden Pact") {
		t.Error("player view: filtered dm_only event must not appear")
	}
}

// TestLedgerView_EmptyState: a year with no events shows the empty chronicle
// message, not an empty box.
func TestLedgerView_EmptyState(t *testing.T) {
	html := renderLedger(t, ledgerTestData(1492, nil))
	if !strings.Contains(html, "No recorded events in 1492 DR") {
		t.Errorf("expected empty-state copy; got:\n%s", html)
	}
}

// TestLedgerView_YearNav: the Ledger's prev/next navigation is labelled by YEAR
// (so the j/k shortcut's 'Previous year'/'Next year' hooks resolve). The nav
// moved OUT of the ledger card and into the shared sticky command bar
// (C-CAL-DESIGN-PASS-1 §1) — the aria-labels must render there, driven by the
// active view ("ledger" → year).
func TestLedgerView_YearNav(t *testing.T) {
	data := ledgerTestData(1492, nil)
	data.ActiveCalendar = ledgerTestCalendar() // header nav renders only with an active calendar
	var sb strings.Builder
	if err := calendarV2Header(nil, data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render command bar: %v", err)
	}
	html := sb.String()
	for _, want := range []string{`aria-label="Previous year"`, `aria-label="Next year"`} {
		if !strings.Contains(html, want) {
			t.Errorf("command bar (ledger view) nav missing %q", want)
		}
	}
}

// TestLedgerYearWindowEnd_LeapAware pins the handler's one-year SQL window
// boundary (the coordinator-review defect): a calendar whose LAST month gains
// LeapYearDays must extend the window end on leap years, or events stored on
// the trailing leap days silently vanish from the Ledger while ledgerRows
// still iterates those days.
func TestLedgerYearWindowEnd_LeapAware(t *testing.T) {
	cal := ledgerTestCalendar()
	last := len(cal.Months) - 1
	cal.Months[last].LeapYearDays = 2
	cal.LeapYearEvery = 4
	cal.LeapYearOffset = 0

	// Non-leap year: base days only.
	m, d := ledgerYearWindowEnd(cal, 1491)
	if m != len(cal.Months) || d != cal.Months[last].Days {
		t.Fatalf("non-leap: expected (%d,%d); got (%d,%d)", len(cal.Months), cal.Months[last].Days, m, d)
	}

	// Leap year: the trailing leap days are inside the window.
	m, d = ledgerYearWindowEnd(cal, 1492)
	want := cal.Months[last].Days + 2
	if m != len(cal.Months) || d != want {
		t.Fatalf("leap: expected (%d,%d); got (%d,%d)", len(cal.Months), want, m, d)
	}

	// Degenerate: no months.
	empty := &Calendar{}
	m, d = ledgerYearWindowEnd(empty, 1491)
	if m != 0 || d != 1 {
		t.Fatalf("empty calendar: expected (0,1); got (%d,%d)", m, d)
	}
}
