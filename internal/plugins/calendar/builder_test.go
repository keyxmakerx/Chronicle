// builder_test.go — the wizard's Go contract.
//
// The assertions here are about the two things the dispatch says are worth
// getting exactly right and cannot be checked by looking at a screenshot:
//
//  1. THE PREVIEW IS THE SHIPPED BLOCK. Not "renders similarly to" — the same
//     projectBlock, the same buildMonthGeometry, the same blockDateLine faults,
//     from a *Calendar that has no row behind it.
//  2. THE PREVIEW CANNOT BE ASKED FOR ANYTHING IT SHOULD NOT GIVE. The bounds
//     reject rather than truncate, and every one of them is cross-checked
//     against what the terminal submit will actually accept.
package calendar

import (
	"context"
	"strings"
	"testing"

	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// builderStubService is a calendarService over the package's mock repository,
// used ONLY to run the shipped Set* validators. Every assertion that reaches it
// is a CROSS-CHECK — "the wizard cannot preview a calendar the terminal submit
// will then reject" (dispatch §7.3) — so it must be the real code and not a
// restatement of its bounds.
func builderStubService() *calendarService {
	return &calendarService{repo: &mockCalendarRepo{}, events: NoopCalendarEventPublisher{}}
}

// builderCountCells counts the IN-RANGE days the geometry laid out. Lead and
// trail cells carry Day == 0 and are not days of this month.
func builderCountCells(d calblock.BlockData) int {
	n := 0
	for _, row := range d.Month.Rows {
		for _, c := range row.Cells {
			if c.Day > 0 {
				n++
			}
		}
	}
	return n
}

// fxBuilderDraft is Harptos as the wizard holds it — the signed demo calendar,
// carried as draft state rather than as a row.
//
// today IS NOT CARRIED, and its absence is the ruling rather than an omission
// ([WZ-15] item 4). ApplyImport forces CurrentMonth = 1, CurrentDay = 1 and no
// service method sets the date in the same transaction, so a preset that
// authored `today: 14 Hammer` would draw a today pill the created calendar does
// not have. Preview and result match instead, which is what the operator's own
// signed condition — "the result looks the same" — actually demands.
func fxBuilderDraft() *builderDraft {
	d := &builderDraft{
		Preset: "harptos", Mode: ModeFantasy, Name: "Harptos of Imix",
		EpochName: "RoW", Year: 1523,
		Weekdays:  []string{"Sar", "Mol", "Zor", "Wir", "Nym", "Lyr", "Tam", "Kes", "Vel", "Odd"},
		LeapEvery: 4, LeapAdd: 1, LeapName: "Shieldmeet", LeapAfter: "Midsummer",
		Hue: "harptos", Pattern: "p1", Letter: "H",
	}
	for _, n := range []string{"Hammer", "Alturiak", "Ches", "Tarsakh", "Mirtul", "Kythorn",
		"Flamerule", "Eleasis", "Eleint", "Marpenoth", "Uktar", "Nightal"} {
		d.Months = append(d.Months, builderMonth{Name: n, Days: 30})
	}
	d.Moons = []builderMoon{
		{Name: "Alder", Period: 31.4, NewAt: 12},
		{Name: "Umber", Period: 46.5, NewAt: -4.25},
		{Name: "Flint", Period: 11.3, NewAt: 3},
		{Name: "Sable", Period: 88.2, NewAt: 41},
	}
	d.Seasons = []builderSeason{
		{Name: "Long Night", ColorName: "frost", Color: "oklch(0.65 0.05 240)",
			StartMonth: 12, StartDay: 1, EndMonth: 2, EndDay: 30},
	}
	d.Eras = []builderEra{
		{Name: "Reckoning of Wards", Code: "RoW", ColorName: "ward",
			Color: "oklch(0.55 0.09 210)", StartYear: 1},
	}
	return d
}

// ── the stations ────────────────────────────────────────────────────────────

func TestBuilderStations_NineOfThem(t *testing.T) {
	if len(builderStations) != 9 {
		t.Fatalf("the wizard has NINE stations (Start + 8), got %d", len(builderStations))
	}
	if builderStations[0].Name != "Start" {
		t.Errorf("station 0 is Start, got %q", builderStations[0].Name)
	}
	if last := builderStations[len(builderStations)-1]; last.Key != "review" {
		t.Errorf("the last station is Review, got %q", last.Key)
	}
}

// TestBuilderStationIndex_RejectsRatherThanClamps pins [LYR-4]'s pattern on the
// one query parameter this surface accepts. A ?step= value that quietly slid to
// Start would turn a wrong bookmark into a wrong page with no complaint.
func TestBuilderStationIndex_RejectsRatherThanClamps(t *testing.T) {
	if i, ok := builderStationIndex(""); !ok || i != 0 {
		t.Errorf("an absent ?step= is Start: got (%d,%v)", i, ok)
	}
	if i, ok := builderStationIndex("eras"); !ok || i != 7 {
		t.Errorf("eras is station 7: got (%d,%v)", i, ok)
	}
	for _, bad := range []string{"9", "Start", "moons ", "../../etc", "structure;drop"} {
		if _, ok := builderStationIndex(bad); ok {
			t.Errorf("%q was accepted as a station key — unknown values are a 400, not a clamp", bad)
		}
	}
}

// ── the bounds ──────────────────────────────────────────────────────────────

// TestBuilderValidate_BoundsRejectRatherThanTruncate walks §7.3's table.
func TestBuilderValidate_BoundsRejectRatherThanTruncate(t *testing.T) {
	long := strings.Repeat("m", builderLimits.MaxNameRunes+1)

	cases := []struct {
		name string
		bend func(d *builderDraft)
		want string
	}{
		{"a bad mode", func(d *builderDraft) { d.Mode = "wizardly" }, "mode"},
		{"too many months", func(d *builderDraft) {
			d.Months = make([]builderMonth, builderLimits.MaxMonths+1)
			for i := range d.Months {
				d.Months[i] = builderMonth{Name: "M", Days: 1}
			}
		}, "months"},
		{"a month past the SetMonths ceiling", func(d *builderDraft) {
			d.Months[0].Days = builderLimits.MaxMonthDays + 1
		}, "days must be between"},
		{"a negative month", func(d *builderDraft) { d.Months[0].Days = -1 }, "days must be between"},
		{"no weekdays at all", func(d *builderDraft) { d.Weekdays = nil }, "week must have"},
		{"too many weekdays", func(d *builderDraft) {
			d.Weekdays = make([]string, builderLimits.MaxWeekdays+1)
		}, "week must have"},
		{"too many moons", func(d *builderDraft) {
			d.Moons = make([]builderMoon, builderLimits.MaxMoons+1)
		}, "moons"},
		{"a moon with no cycle", func(d *builderDraft) { d.Moons[0].Period = 0 }, "period must be positive"},
		{"too many seasons", func(d *builderDraft) {
			d.Seasons = make([]builderSeason, builderLimits.MaxSeasons+1)
		}, "seasons"},
		{"too many eras", func(d *builderDraft) {
			d.Eras = make([]builderEra, builderLimits.MaxEras+1)
		}, "eras"},
		{"an over-long month name", func(d *builderDraft) { d.Months[0].Name = long }, "longer than"},
		{"an over-long calendar name", func(d *builderDraft) { d.Name = long }, "longer than"},
		{"an over-long era code", func(d *builderDraft) { d.Eras[0].Code = long }, "longer than"},
		{"an absurd leap interval", func(d *builderDraft) {
			d.LeapEvery = builderLimits.MaxLeapEvery + 1
		}, "leap interval"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := fxBuilderDraft()
			tc.bend(d)
			err := validateBuilderDraft(d)
			if err == nil {
				t.Fatalf("%s was accepted — the bounds REJECT, they never truncate", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name %q — a rejection must say which bound it is", err, tc.want)
			}
		})
	}

	if err := validateBuilderDraft(fxBuilderDraft()); err != nil {
		t.Fatalf("the signed demo calendar must validate: %v", err)
	}
}

// TestBuilderValidate_MonthCeilingMatchesTheSubmit is the cross-check §7.3
// demands by name: "the wizard cannot preview a calendar the terminal submit
// will then reject. A divergence between the two is a bug you own."
//
// SetMonths (service.go:1334) refuses `Days < 1 || Days > 400`, so the preview
// ceiling is 400 and NOT the dispatch's tabled 1000. A 999-day month would
// preview beautifully and then be refused at Create, which is exactly the
// divergence the instruction forbids.
func TestBuilderValidate_MonthCeilingMatchesTheSubmit(t *testing.T) {
	if builderLimits.MaxMonthDays != 400 {
		t.Fatalf("the preview ceiling is SetMonths' ceiling; got %d, want 400",
			builderLimits.MaxMonthDays)
	}
	// Exercised through the SHIPPED SetMonths, over the package's mock repo, so
	// the pin cannot drift from the code it claims to agree with.
	svc := builderStubService()
	ctx := context.Background()
	if err := svc.SetMonths(ctx, "c", []MonthInput{{Name: "M", Days: builderLimits.MaxMonthDays}}); err != nil {
		t.Errorf("a month at the preview ceiling must be creatable: %v", err)
	}
	if err := svc.SetMonths(ctx, "c", []MonthInput{{Name: "M", Days: builderLimits.MaxMonthDays + 1}}); err == nil {
		t.Error("a month one past the preview ceiling must be refused at submit — " +
			"if it is not, the two bounds have drifted apart")
	}
}

// TestBuilderValidate_ZeroDaysIsPreviewableAndUncreatable pins the one
// deliberate asymmetry: a 0-day month IS the fault state, so it must preview
// (that is where the shipped fault text renders) and must not create.
func TestBuilderValidate_ZeroDaysIsPreviewableAndUncreatable(t *testing.T) {
	d := fxBuilderDraft()
	d.Months[2].Days = 0

	if err := validateBuilderDraft(d); err != nil {
		t.Fatalf("a 0-day month must PREVIEW — it is the fault state: %v", err)
	}
	if blocked := builderCreateBlocked(d); blocked == "" {
		t.Error("a 0-day month must hold Create")
	} else if !strings.Contains(blocked, "Ches") {
		t.Errorf("the block reason must name the month: %q", blocked)
	}
	svc := builderStubService()
	if err := svc.SetMonths(context.Background(), "c", []MonthInput{{Name: "Ches", Days: 0}}); err == nil {
		t.Error("the SERVER must refuse a 0-day month too — the honesty state is " +
			"enforced by the model, not offered by the client")
	}
}

// TestBuilderValidate_CellCeiling — AND THE FINDING THAT IT IS UNREACHABLE.
//
// The dispatch tables a 5000-cell ceiling as the bound that answers §7.3's real
// risk: blockRowCount computes from lead/days/weekLen with no cap anywhere in
// the geometry, so a body claiming 500 months × 999 days would build a very
// large DOM. That risk is real and the ceiling is right to exist — but with the
// OTHER bounds in force it can never fire, and this test says so out loud
// rather than leaving a reader to assume it is load-bearing.
//
// The preview renders ONE month, so the cost is the worst single month:
// (MaxMonthDays + MaxMonthDays leap) rounded up to whole weeks of MaxWeekdays =
// 800 days → 25 rows → 800 cells. The month COUNT never multiplies it. So the
// effective ceiling today is the day/weekday pair, and 5000 is defence in depth
// against a later slice widening one of them without re-deriving this.
func TestBuilderValidate_CellCeiling(t *testing.T) {
	// The arithmetic itself: the worst month, rounded up to whole weeks.
	c := fxBuilderDraft()
	c.Weekdays = c.Weekdays[:7]
	c.Months = []builderMonth{{Name: "A", Days: 30}, {Name: "B", Days: 31}}
	if got, want := builderCellCount(c), 35; got != want {
		t.Errorf("cell count = %d, want %d (31 days over a 7-day week is 5 rows)", got, want)
	}

	// THE WORST CASE THE OTHER BOUNDS ALLOW, built at its own limits.
	worst := &builderDraft{
		Mode: ModeFantasy, Name: "worst case", Year: 1,
		Months:   make([]builderMonth, builderLimits.MaxMonths),
		Weekdays: make([]string, builderLimits.MaxWeekdays),
		Eras:     []builderEra{{Name: "E", Code: "E", StartYear: 1}},
	}
	for i := range worst.Weekdays {
		worst.Weekdays[i] = "d"
	}
	for i := range worst.Months {
		worst.Months[i] = builderMonth{
			Name: "M", Days: builderLimits.MaxMonthDays, LeapDays: builderLimits.MaxMonthDays,
		}
	}
	cells := builderCellCount(worst)
	if cells > builderLimits.MaxCells {
		t.Fatalf("worst case is %d cells, past the %d ceiling — then the ceiling IS "+
			"reachable and this test's comment is wrong", cells, builderLimits.MaxCells)
	}
	if err := validateBuilderDraft(worst); err != nil {
		t.Fatalf("the worst case the other bounds allow must still validate "+
			"(%d cells): %v", cells, err)
	}
	t.Logf("worst case the other bounds allow: %d cells against a %d ceiling — "+
		"the ceiling is defence in depth, not the operative bound", cells, builderLimits.MaxCells)

	// And the ceiling still FIRES, so it is not a guard that cannot fail: the
	// function is exercised directly at a shape the list bounds would refuse.
	huge := &builderDraft{Weekdays: make([]string, 40), Months: []builderMonth{{Days: 9999}}}
	if got := builderCellCount(huge); got <= builderLimits.MaxCells {
		t.Errorf("the ceiling never fires even on an unbounded structure (%d cells)", got)
	}
}

// ── the draft → *Calendar producer ──────────────────────────────────────────

// TestDraftCalendar_ShapeIsTheOneTheDispatchNames.
func TestDraftCalendar_ShapeIsTheOneTheDispatchNames(t *testing.T) {
	cal := draftCalendar(fxBuilderDraft())

	if cal.ID != "draft" {
		t.Errorf("the draft carries a stable pseudo-id; got %q", cal.ID)
	}
	if cal.CampaignID != "" {
		t.Errorf("CampaignID is EMPTY so the draft gets no switchboard; got %q", cal.CampaignID)
	}
	if cal.CurrentMonth != 1 || cal.CurrentDay != 1 {
		t.Errorf("the draft opens on 1/1 because ApplyImport forces it; got %d/%d",
			cal.CurrentMonth, cal.CurrentDay)
	}
	if len(cal.Months) != 12 || len(cal.Weekdays) != 10 || len(cal.Moons) != 4 {
		t.Errorf("sub-resources did not carry: %d months, %d weekdays, %d moons",
			len(cal.Months), len(cal.Weekdays), len(cal.Moons))
	}
	for i, m := range cal.Months {
		if m.SortOrder != i {
			t.Errorf("month %d has sort order %d — the list IS the order", i, m.SortOrder)
		}
	}
	if cal.EpochName == nil || *cal.EpochName != "RoW" {
		t.Error("the epoch name carries; it is what the year line reads")
	}
}

// TestBuilderPreview_IsTheShippedBlock is the wave's most consequential
// assertion. If it ever fails by being deleted, Chronicle has two month
// renderers.
func TestBuilderPreview_IsTheShippedBlock(t *testing.T) {
	d := fxBuilderDraft()
	got := builderPreviewBlock(d, 0)

	// The geometry came from buildMonthGeometry, through Calendar.MonthDays.
	if got.Month.WeekLen != 10 {
		t.Errorf("week length is DATA and the grid reads it: got %d", got.Month.WeekLen)
	}
	if got.Month.Name != "Hammer" {
		t.Errorf("month 0 is Hammer; got %q", got.Month.Name)
	}
	if n := builderCountCells(got); n == 0 {
		t.Fatal("the preview built no cells — the real geometry did not run")
	}
	if got.CalendarSlug != "draft" {
		t.Errorf("the ANSWER-key namespace is the draft slug; got %q", got.CalendarSlug)
	}
	if got.DateLabel == "" || got.Fault != "" {
		t.Errorf("a complete draft resolves a date: label=%q fault=%q", got.DateLabel, got.Fault)
	}
}

// TestBuilderPreview_HostDecisionsAreZeroTogether pins [WZ-2c] and, through it,
// r54's invariant: HasSwitchboard == (PersistURL != ""). Both are zero HERE by
// construction — the draft has an empty campaign, blockPrefsPath returns "" for
// one, and nothing in the wizard assigns either field. That is the difference
// between an invariant that holds and one somebody remembers to hold.
func TestBuilderPreview_HostDecisionsAreZeroTogether(t *testing.T) {
	got := builderPreviewBlock(fxBuilderDraft(), 0)

	if got.Layers.HasSwitchboard {
		t.Error("a calendar that does not exist has nowhere to persist a layer set")
	}
	if got.Layers.PersistURL != "" {
		t.Errorf("PersistURL must be empty with HasSwitchboard false (r54); got %q",
			got.Layers.PersistURL)
	}
	if len(got.Layers.Enabled) != 1 || got.Layers.Enabled[0] != "moons" {
		t.Errorf("the wizard is a HOST and passes DEF — a month with its moon phases "+
			"and nothing else; got %v", got.Layers.Enabled)
	}
}

// TestBuilderPreview_ADraftHasNoEvents. Events: nil is correct AND safe, and a
// zero here is honest rather than a stub: a calendar that does not exist has no
// events.
func TestBuilderPreview_ADraftHasNoEvents(t *testing.T) {
	got := builderPreviewBlock(fxBuilderDraft(), 0)
	for _, row := range got.Month.Rows {
		for _, c := range row.Cells {
			if len(c.Marks) != 0 {
				t.Fatalf("day %d carries %d marks — a draft has no events", c.Day, len(c.Marks))
			}
		}
	}
	if got.Viewer.HiddenCount != 0 {
		t.Errorf("a draft hides nothing; got %d", got.Viewer.HiddenCount)
	}
}

// TestBuilderPreview_TheShippedFaultTextIsNotReimplemented. The dispatch is
// explicit: a wizard on station 1 with no months renders blockDateLine's OWN
// fault, and there is no bespoke "nothing to preview yet" placeholder anywhere.
func TestBuilderPreview_TheShippedFaultTextIsNotReimplemented(t *testing.T) {
	d := fxBuilderDraft()
	d.Months = nil

	got := builderPreviewBlock(d, 0)
	if got.Fault != "Needs months — 0 months defined, dates cannot resolve" {
		t.Errorf("the fault text must come from blockDateLine unmodified; got %q", got.Fault)
	}
	if got.DateLabel != "" {
		t.Errorf("the fault prints WHERE THE DATE WOULD GO and no date element is "+
			"emitted; got label %q", got.DateLabel)
	}

	// A half-built calendar draws in the NAMED 7-column fallback with a comment
	// saying why — blockWeekLen's blockDefaultWeekLen, not an invented shape.
	noWeek := fxBuilderDraft()
	noWeek.Weekdays = nil
	if w := builderPreviewBlock(noWeek, 0).Month.WeekLen; w != blockDefaultWeekLen {
		t.Errorf("a weekday-less draft falls back to the named default %d; got %d",
			blockDefaultWeekLen, w)
	}
}

// TestBuilderPreview_ZeroDayMonthDrawsRatherThanPanics. The fault state is a
// state the surface must be able to RENDER, not merely to refuse.
func TestBuilderPreview_ZeroDayMonthDrawsRatherThanPanics(t *testing.T) {
	d := fxBuilderDraft()
	d.Months[0].Days = 0

	got := builderPreviewBlock(d, 0)
	if got.Fault == "" {
		t.Error("a 0-day CURRENT month cannot resolve a date and the Block must say so")
	}
	if n := builderCountCells(got); n != 0 {
		t.Errorf("a 0-day month draws no in-range cells; got %d", n)
	}

	// The pager may be parked anywhere, including past the end of a structure
	// the author just shortened. That is a UI position, not authored data.
	if out := builderPreviewBlock(d, 99); out.Month.Name != "Hammer" {
		t.Errorf("an out-of-range pager cursor clamps to the first month; got %q", out.Month.Name)
	}
}

// ── the derived facts the stations print ────────────────────────────────────

func TestBuilderDerivedFacts(t *testing.T) {
	d := fxBuilderDraft()
	d.Months = append(d.Months, builderMonth{Name: "Midwinter", Days: 1, Intercalary: true})

	if got, want := builderMonthDays(d), 360; got != want {
		t.Errorf("month days = %d, want %d", got, want)
	}
	if got, want := builderIntercalaryDays(d), 1; got != want {
		t.Errorf("intercalary days = %d, want %d", got, want)
	}
	if got, want := builderYearDays(d), 361; got != want {
		t.Errorf("year days = %d, want %d", got, want)
	}

	// The leap list is DERIVED from the modulus, never authored year by year.
	if got := builderNextLeaps(d, 4); len(got) != 4 || got[0] != 1524 || got[3] != 1536 {
		t.Errorf("next leap years = %v, want 1524 · 1528 · 1532 · 1536", got)
	}
	none := fxBuilderDraft()
	none.LeapEvery, none.LeapAdd = 0, 0
	if got := builderNextLeaps(none, 4); got != nil {
		t.Errorf("a calendar with no leap rule has no next leap years; got %v", got)
	}
}

// TestBuilderWeekSplit_NothingKnowsSeven. The split is generalised from the
// signed "week of ten" case to any even week length ≥ 8; a literal 7 is legal
// as preset DATA and illegal as layout ([WZ-11a] SIGNED).
func TestBuilderWeekSplit_NothingKnowsSeven(t *testing.T) {
	for week, want := range map[int]int{3: 0, 6: 0, 7: 0, 8: 4, 9: 0, 10: 5, 12: 6} {
		if got := builderWeekSplit(week); got != want {
			t.Errorf("split at week %d = %d, want %d", week, got, want)
		}
	}
}

// ── the create path ─────────────────────────────────────────────────────────

// TestBuilderImportResult_IsTheImporterPath pins the ruling that collapses two
// features into one: a preset and an import land through the SAME
// *ImportResult and therefore the same ApplyImport. No preset table, no second
// apply path, no new parser.
func TestBuilderImportResult_IsTheImporterPath(t *testing.T) {
	d := fxBuilderDraft()
	d.Months = append(d.Months, builderMonth{Name: "Midwinter", Days: 1, Intercalary: true})

	res := builderImportResult(d)
	if res.Format != FormatChronicle {
		t.Errorf("a wizard draft is Chronicle-native; got %q", res.Format)
	}
	if res.CalendarName != "Harptos of Imix" {
		t.Errorf("the name carries; got %q", res.CalendarName)
	}
	if len(res.Months) != 13 || len(res.Weekdays) != 10 ||
		len(res.Moons) != 4 || len(res.Eras) != 1 {
		t.Fatalf("sub-resources did not carry: %d/%d/%d/%d",
			len(res.Months), len(res.Weekdays), len(res.Moons), len(res.Eras))
	}
	if !res.Months[12].IsIntercalary {
		t.Error("an intercalary day is a Month with IsIntercalary — the wizard invents no second shape")
	}
	if res.Settings.EpochName == nil || *res.Settings.EpochName != "RoW" {
		t.Error("the epoch rides the settings, as it does from every parser")
	}
	if res.Settings.LeapYearEvery != 4 {
		t.Errorf("the single-modulus leap rule carries; got %d", res.Settings.LeapYearEvery)
	}

	// What the wizard hands ApplyImport must survive the SAME validation the
	// four parsers' output survives.
	svc, ctx := builderStubService(), context.Background()
	if err := svc.SetMonths(ctx, "c", res.Months); err != nil {
		t.Errorf("the produced months must pass the shipped validator: %v", err)
	}
	if err := svc.SetWeekdays(ctx, "c", res.Weekdays); err != nil {
		t.Errorf("the produced weekdays must pass the shipped validator: %v", err)
	}
	if err := svc.SetMoons(ctx, "c", res.Moons); err != nil {
		t.Errorf("the produced moons must pass the shipped validator: %v", err)
	}
	if err := svc.SetEras(ctx, "c", res.Eras); err != nil {
		t.Errorf("the produced eras must pass the shipped validator: %v", err)
	}
}

// TestBuilderCreateBlocked_TheEraGateIsWizardLocalPolicy pins [WZ-3] SIGNED —
// including its copy consequence, which is the part an executor is most likely
// to get wrong.
//
// Chronicle has NO era-relative year numbering: a zero-era calendar resolves
// "Deepwinter 14, 1523" perfectly well, which is why wave 1 refused to
// synthesise an era fault and flagged the question upward. The wizard may
// decline to CREATE what would read ambiguously — but it must say so as a claim
// about ITSELF, never as a claim about the model.
func TestBuilderCreateBlocked_TheEraGateIsWizardLocalPolicy(t *testing.T) {
	ok := fxBuilderDraft()
	if got := builderCreateBlocked(ok); got != "" {
		t.Errorf("a complete declaration creates; got %q", got)
	}

	noEra := fxBuilderDraft()
	noEra.Eras = nil
	msg := builderCreateBlocked(noEra)
	if msg == "" {
		t.Fatal("the wizard holds Create until an era exists")
	}
	if !strings.Contains(msg, "this calendar has no era") {
		t.Errorf("the copy must be a claim about the WIZARD; got %q", msg)
	}
	for _, false_ := range []string{"cannot resolve a year", "dates cannot resolve"} {
		if strings.Contains(strings.ToLower(msg), false_) {
			t.Errorf("the copy claims %q — that is a claim about the MODEL and it is "+
				"FALSE: a zero-era calendar still resolves its year. [WZ-3] pre-authorises "+
				"exactly this re-wording", false_)
		}
	}

	// And the Block agrees: it renders a zero-era calendar's date without a
	// fault, which is the evidence the gate is policy and not physics.
	if got := builderPreviewBlock(noEra, 0); got.Fault != "" {
		t.Errorf("the BLOCK resolves a zero-era date — the gate is the wizard's, not "+
			"the model's; got fault %q", got.Fault)
	}
}
