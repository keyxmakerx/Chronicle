// builder.go — THE BUILDER WIZARD's view models and its draft→*Calendar
// producer (calendar-v4 wave 4, W-H, C-CALV4-WIZARD-P13).
//
// ── THE ONE CONSEQUENTIAL RULING IN THE WAVE ([WZ-2] SIGNED) ───────────────
//
// The live month preview is THE SHIPPED BLOCK. It is not a second month
// renderer, and there is no second producer:
//
//	wizard form state → *Calendar (in memory, NEVER persisted)
//	                  → projectBlock(BlockProjectionInput{Calendar: draft, …})
//	                  → calblock.BlockData
//	                  → calblock.Block(d)          ← the REAL renderer
//
// Every link is already pure. calblock.Block "performs no queries and reads no
// request state" (block.templ:31). projectBlock is
// `func(BlockProjectionInput) calblock.BlockData` — no context, no repository,
// no DB — and the only DB work in the Block path is BlockService.Block doing
// requireVisibleCalendar → candidateEvents → tiedEventIDs → CalendarLinkStatus
// BEFORE it calls projectBlock. A draft skips all four. Geometry is likewise
// pure: buildMonthGeometry feeds off Calendar.MonthDays, blockWeekLen,
// blockWeekdays, moonDiscsForDay, blockEraBands, blockIntercalary.
//
// WHAT THAT BUYS, AND WHY THE ALTERNATIVE WAS REFUSED. Get this wrong and
// Chronicle has two month renderers forever, and the next geometry change —
// leap-aware day counts, the five-column rule, the era-band Half/Edge
// semantics, the moon cap — must be made twice. A client-side re-render was
// refused on the same evidence from the other direction: the Block's geometry
// is ~700 lines of leap-aware Go deliberately routed through
// Calendar.MonthDays and sharing v2WeekdayIndexFor so that the Block's column,
// V2's column and Event.OccursOn's recurrence stride all agree, pinned by
// TestBlockCounterDivergencePin. Re-implementing it in JS creates a FOURTH
// counter and a silent divergence.
//
// projectBlock's *Calendar is contractually non-nil and until now only ever
// came from the DB. THIS FILE WIDENS ITS PROVENANCE, NOT ITS CONTRACT: the
// draft below is a fully-formed *Calendar that simply has no row behind it.
//
// ── WHAT THE DRAFT IS NOT ─────────────────────────────────────────────────
//
// It is never persisted. There is no drafts table, no Redis scratch key and no
// migration ([WZ-12] SIGNED, dispatch §8.1): draft state lives in the form and
// the ?step= URL, every preview POST rebuilds it from the posted body, and the
// terminal POST creates and applies atomically. That is the ONLY option under
// which the surface's own promise — "Nothing is written until Create on
// Review" — is literally true rather than advertised. It is also the only one
// with no abandoned-draft cleanup story and no egress question: a draft is not
// campaign content and never becomes a row, so it enters no export and no
// AI-workspace DTO BY CONSTRUCTION.
package calendar

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// ── the nine stations ───────────────────────────────────────────────────────

// builderStation is one stop on the rail. NINE STATIONS, presented as Start
// plus eight steps: the wtop reads "Start" at index 0 and "Step N of 8" for
// N≥1. The rail inserts a hairline after Start, separating "choose a shape"
// from "bend it".
type builderStation struct {
	Key  string
	Name string
}

// builderStations is the signed order (mockup STEPS). It is a var only so the
// slice can be ranged; nothing mutates it.
var builderStations = []builderStation{
	{Key: "start", Name: "Start"},
	{Key: "structure", Name: "Structure"},
	{Key: "week", Name: "Week"},
	{Key: "intercalary", Name: "Intercalary"},
	{Key: "leap", Name: "Leap rules"},
	{Key: "moons", Name: "Moons"},
	{Key: "seasons", Name: "Seasons"},
	{Key: "eras", Name: "Eras"},
	{Key: "review", Name: "Review"},
}

// builderStationIndex resolves a ?step= value to a station index.
//
// REJECT, DO NOT CLAMP ([LYR-4]'s pattern, dispatch §7.2). An unknown value is
// a 400 and not a silent slide to Start: a query parameter that quietly means
// something else is how a bookmark becomes a wrong page with no complaint.
func builderStationIndex(key string) (int, bool) {
	if strings.TrimSpace(key) == "" {
		return 0, true
	}
	for i, s := range builderStations {
		if s.Key == key {
			return i, true
		}
	}
	return 0, false
}

// ── the draft ───────────────────────────────────────────────────────────────

// builderMonth is one row of the Structure station, or — with Intercalary set
// — one row of the Intercalary station.
//
// INTERCALARY DAYS ARE MONTHS IN CHRONICLE'S MODEL (Month.IsIntercalary), and
// the wizard does not invent a second shape for them. The mockup authors them
// as `{name, after}`; the model orders them in the same sorted month list,
// which is what blockIntercalary already reads to draw the full-width band row.
// One list, one sort order, one thing to validate.
type builderMonth struct {
	Name        string
	Days        int
	Intercalary bool
	LeapDays    int
}

// builderMoon is one declared body. Period and offset derive every phase, turn
// and node window — nothing is authored date by date.
type builderMoon struct {
	Name   string
	Period float64
	NewAt  float64
}

// builderSeason carries its own AUTHORED colour (Season.Color) and the mono
// colour NAME beside it. The name is the second channel and it is not optional:
// colour alone is never load-bearing anywhere in calendar-v4.
type builderSeason struct {
	Name       string
	ColorName  string
	Color      string
	StartMonth int
	StartDay   int
	EndMonth   int
	EndDay     int
}

// builderEra is one era. StartYear is a YEAR and not a date, which is the model
// on main: Era{StartYear int, EndYear *int}. See builderDraft.Eras for why the
// mockup's mid-month era band is deliberately not reproduced.
type builderEra struct {
	Name      string
	Code      string
	ColorName string
	Color     string
	StartYear int
}

// builderDraft is the wizard's whole state. It is posted as a form, rebuilt
// from the body on every preview, and never persisted.
//
// Mode is fantasy | reallife and reallife is NOT cosmetic ([WZ-5] SIGNED):
// CreateCalendar overrides year/hours/leap from the wall clock and names the
// epoch "AD", seedDefaults seeds twelve Gregorian months and seven weekdays,
// and TracksRealTime + an IANA RealTimeZone ride on it. A "Real world"
// preset that created a FANTASY calendar shaped like Gregorian would be the
// wrong mode with no timezone and no wall-clock authority — and would therefore
// not be the Bench's real-world Block, which the master plan requires the Bench
// to host. Real-life is a first-class Start card that flips Mode and
// short-circuits stations 1–7, exactly as the importer door already
// short-circuits them.
type builderDraft struct {
	Preset    string
	Mode      string
	Name      string
	EpochName string
	Year      int

	Months   []builderMonth
	Weekdays []string
	Moons    []builderMoon
	Seasons  []builderSeason
	Eras     []builderEra

	// LeapEvery/LeapAdd are Chronicle's single-modulus leap model, and it is
	// the WHOLE model: the plugin stores "every N years, add D days", not a
	// list of exceptions. A preset carrying a real-world exception clause
	// ("skip years ×100 unless ×400") shows it, states that it is not editable,
	// and Review says in as many words what Create will actually write.
	LeapEvery int
	LeapAdd   int
	LeapName  string
	LeapAfter string
	LeapNote  string

	// TimeZone is the IANA anchor, real-life only.
	TimeZone string

	// Identity — DERIVED, NOT PERSISTED ([WZ-6] SIGNED). No migration ships:
	// Chronicle has no columns for hue/pattern/letter, and blockCalHue /
	// blockCalPattern assign from mode plus a stable hash of the calendar's
	// UUID. These three are what the WIZARD'S OWN CHROME prints — a preview of
	// the identity the author chose — and Review carries the same `needs
	// backend` chip calendar-settings.html already carries, because the created
	// calendar's real hue comes from an id that does not exist until Create.
	// The Block inside the preview column derives its own from the draft slug
	// and is left alone: it is the real renderer and it is not overridden.
	Hue     string
	Pattern string
	Letter  string
	// HollowSwatch marks Blank, whose identity is UNCHOSEN. There is no
	// --cal-blank token and inventing one is forbidden; Blank falls back to a
	// NEUTRAL RULE token and never to --own-none, because crossing the OWNER
	// axis into calendar identity is the same category error ([WZ-7d]).
	HollowSwatch bool
}

// ── the input bounds (dispatch §7.3) ────────────────────────────────────────

// builderLimits bounds a preview on the way IN, and REJECTS rather than
// truncates.
//
// THE REAL RISK ON THE PREVIEW ROUTE IS NOT XSS — templ escapes by default and
// every authored name is rendered as text. It is grid size: blockRowCount
// computes from lead/days/weekLen with NO CAP anywhere in the geometry, so a
// body claiming 500 months × 999 days would build an enormous DOM. These are
// validation limits on a preview, not product limits.
//
// MaxMonthDays IS 400 AND NOT THE DISPATCH'S 1000, AND THAT IS THE CROSS-CHECK
// THE DISPATCH ASKED FOR. §7.3 requires each bound sanity-checked against what
// SetMonths / SetWeekdays / SetMoons already enforce, "so the wizard cannot
// preview a calendar the terminal submit will then reject. A divergence between
// the two is a bug you own, not a coincidence." SetMonths (service.go:1334)
// enforces `m.Days < 1 || m.Days > 400`. A 1000-day month would preview
// beautifully and then be refused by Create, which is exactly the divergence
// the instruction forbids — so the preview bound is narrowed to the submit
// bound and the number is 400.
//
// ZERO DAYS IS LEGAL IN A PREVIEW AND ILLEGAL AT CREATE, and that asymmetry is
// the design rather than an oversight: a 0-day month IS the fault state, the
// preview must be able to draw it (it is where blockDateLine's shipped fault
// text renders), and both ApplyImport and SetMonths refuse it server-side — so
// the honesty state is enforced by the model and not by a client-side courtesy.
var builderLimits = struct {
	MaxMonths     int
	MinWeekdays   int
	MaxWeekdays   int
	MaxMonthDays  int
	MaxMoons      int
	MaxSeasons    int
	MaxEras       int
	MaxNameRunes  int
	MaxCells      int
	MaxLeapEvery  int
	MaxLeapAdd    int
	MaxUploadSize int64
}{
	MaxMonths:    64,
	MinWeekdays:  1,
	MaxWeekdays:  32,
	MaxMonthDays: 400,
	MaxMoons:     16,
	MaxSeasons:   64,
	MaxEras:      64,
	MaxNameRunes: 128,
	MaxCells:     5000,
	MaxLeapEvery: 10000,
	MaxLeapAdd:   400,
	// The shipped cap on BOTH import transports (handler.go:1331 and
	// calendar.templ's multipart form). Kept, on both, because the importer's
	// front door is the same parser behind a route without a :calId — nothing
	// about moving the door changes what a body may weigh.
	MaxUploadSize: 10 * 1024 * 1024,
}

// builderName trims and bounds one authored string.
func builderName(field, s string, i int) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > builderLimits.MaxNameRunes {
		return "", apperror.NewValidation(fmt.Sprintf(
			"%s %d: name is longer than %d characters", field, i+1, builderLimits.MaxNameRunes))
	}
	return s, nil
}

// validateBuilderDraft enforces every bound above, REJECT-DON'T-TRUNCATE.
//
// It runs on the preview route AND again on the create route. The preview's
// validation is NOT an authorisation and a caller can skip the preview
// entirely, so the terminal submit re-validates from scratch rather than
// trusting that a preview happened.
func validateBuilderDraft(d *builderDraft) error {
	if d.Mode != ModeFantasy && d.Mode != ModeRealLife {
		return apperror.NewValidation("mode must be fantasy or reallife")
	}
	name, err := builderName("calendar", d.Name, 0)
	if err != nil {
		return err
	}
	d.Name = name

	if len(d.Months) > builderLimits.MaxMonths {
		return apperror.NewValidation(fmt.Sprintf(
			"a calendar may declare at most %d months, including intercalary days", builderLimits.MaxMonths))
	}
	for i := range d.Months {
		if d.Months[i].Name, err = builderName("month", d.Months[i].Name, i); err != nil {
			return err
		}
		if d.Months[i].Days < 0 || d.Months[i].Days > builderLimits.MaxMonthDays {
			return apperror.NewValidation(fmt.Sprintf(
				"month %d: days must be between 0 and %d", i+1, builderLimits.MaxMonthDays))
		}
		if d.Months[i].LeapDays < 0 || d.Months[i].LeapDays > builderLimits.MaxMonthDays {
			return apperror.NewValidation(fmt.Sprintf(
				"month %d: leap days must be between 0 and %d", i+1, builderLimits.MaxMonthDays))
		}
	}

	if len(d.Weekdays) < builderLimits.MinWeekdays || len(d.Weekdays) > builderLimits.MaxWeekdays {
		return apperror.NewValidation(fmt.Sprintf(
			"a week must have between %d and %d days", builderLimits.MinWeekdays, builderLimits.MaxWeekdays))
	}
	for i := range d.Weekdays {
		if d.Weekdays[i], err = builderName("weekday", d.Weekdays[i], i); err != nil {
			return err
		}
	}

	if len(d.Moons) > builderLimits.MaxMoons {
		return apperror.NewValidation(fmt.Sprintf("at most %d moons", builderLimits.MaxMoons))
	}
	for i := range d.Moons {
		if d.Moons[i].Name, err = builderName("moon", d.Moons[i].Name, i); err != nil {
			return err
		}
		// SetMoons (service.go:1372) requires a positive cycle. Match it here
		// so a previewed moon is a creatable one.
		if d.Moons[i].Period <= 0 {
			return apperror.NewValidation(fmt.Sprintf("moon %d: period must be positive", i+1))
		}
	}

	if len(d.Seasons) > builderLimits.MaxSeasons {
		return apperror.NewValidation(fmt.Sprintf("at most %d seasons", builderLimits.MaxSeasons))
	}
	for i := range d.Seasons {
		if d.Seasons[i].Name, err = builderName("season", d.Seasons[i].Name, i); err != nil {
			return err
		}
	}

	if len(d.Eras) > builderLimits.MaxEras {
		return apperror.NewValidation(fmt.Sprintf("at most %d eras", builderLimits.MaxEras))
	}
	for i := range d.Eras {
		if d.Eras[i].Name, err = builderName("era", d.Eras[i].Name, i); err != nil {
			return err
		}
		if d.Eras[i].Code, err = builderName("era code", d.Eras[i].Code, i); err != nil {
			return err
		}
	}

	if d.LeapEvery < 0 || d.LeapEvery > builderLimits.MaxLeapEvery {
		return apperror.NewValidation("leap interval out of range")
	}
	if d.LeapAdd < 0 || d.LeapAdd > builderLimits.MaxLeapAdd {
		return apperror.NewValidation("leap day count out of range")
	}

	// THE CELL CEILING, and it is the bound that actually protects the server.
	// Every other limit above bounds a list; this one bounds the DOM the
	// geometry will build, which is the product of two of them.
	if cells := builderCellCount(d); cells > builderLimits.MaxCells {
		return apperror.NewValidation(fmt.Sprintf(
			"this structure would draw %d cells; the preview is bounded at %d", cells, builderLimits.MaxCells))
	}
	return nil
}

// builderCellCount is the worst-case grid the preview would build: the longest
// month, rounded up to whole weeks, is what a single rendered month costs, and
// the pager can reach every month — so the bound is taken over all of them.
func builderCellCount(d *builderDraft) int {
	week := len(d.Weekdays)
	if week < 1 {
		week = blockDefaultWeekLen
	}
	worst := 0
	for _, m := range d.Months {
		if m.Intercalary {
			continue
		}
		days := m.Days + m.LeapDays
		rows := (days + week - 1) / week
		if c := rows * week; c > worst {
			worst = c
		}
	}
	return worst
}

// ── the draft → *Calendar producer ──────────────────────────────────────────

// builderDraftID is the draft's stable pseudo-id, and it is load-bearing in
// three ways at once.
//
// blockCalendarSlug returns cal.ID verbatim and the widget's DOM instance
// tokens are minted from it, so an EMPTY id yields empty tokens. A constant id
// gives deterministic tokens, a deterministic identity triple (blockCalHue and
// blockCalPattern hash the id), and therefore a REPRODUCIBLE fidelity gate.
//
// It also namespaces the preview's ANSWER keys. Guard B4 wants every dated node
// to carry data-day and the Block emits it; the wizard does not strip it,
// because stripping would mean forking the Block's templ to make a guard's
// premise prettier ([WZ-2b] SIGNED). The honest reading is that data-day is an
// ANSWER KEY NAMESPACE and not a promise that a click resolves — wave 1 emitted
// keys for a consumer that did not exist, and LAYERS-P9 §8.2 did it again. Keys
// minted under this slug cannot collide with any real calendar's, because no
// calendar row has this id.
const builderDraftID = "draft"

// draftCalendar turns the wizard's form state into an in-memory *Calendar.
//
// IT IS NEVER PERSISTED AND IT NEVER TOUCHES A REPOSITORY. CampaignID is
// deliberately empty: blockPrefsPath returns "" for an empty campaign, so the
// draft gets NO switchboard — which is exactly right, because a calendar that
// does not exist has nowhere to persist a layer preference to, and r54 pins
// HasSwitchboard == (PersistURL != ""), so both are zero together and the
// invariant holds BY CONSTRUCTION rather than by an assignment somebody could
// forget.
//
// The date is 1/1 and that is not a simplification: ApplyImport FORCES
// CurrentMonth = 1, CurrentDay = 1 (service.go:2497), and there is no service
// method that sets a calendar's current date in the SAME transaction (SetDate
// exists but is a separate call, and bolting a third call onto Create would
// break the "Create is one act" atomicity the whole surface promises). So the
// preview draws day 1 and the created calendar opens on day 1: preview and
// result MATCH, which is what "the result looks the same" actually demands
// ([WZ-15] item 4, pre-authorised).
func draftCalendar(d *builderDraft) *Calendar {
	cal := &Calendar{
		ID:               builderDraftID,
		CampaignID:       "",
		Mode:             d.Mode,
		Name:             d.Name,
		CurrentYear:      d.Year,
		CurrentMonth:     1,
		CurrentDay:       1,
		HoursPerDay:      24,
		MinutesPerHour:   60,
		SecondsPerMinute: 60,
		LeapYearEvery:    d.LeapEvery,
		Visibility:       "everyone",
	}
	if epoch := strings.TrimSpace(d.EpochName); epoch != "" {
		cal.EpochName = &epoch
	}
	for i, m := range d.Months {
		cal.Months = append(cal.Months, Month{
			CalendarID:    builderDraftID,
			Name:          m.Name,
			Days:          m.Days,
			SortOrder:     i,
			IsIntercalary: m.Intercalary,
			LeapYearDays:  m.LeapDays,
		})
	}
	for i, w := range d.Weekdays {
		cal.Weekdays = append(cal.Weekdays, Weekday{
			CalendarID: builderDraftID, Name: w, SortOrder: i,
		})
	}
	for _, m := range d.Moons {
		cal.Moons = append(cal.Moons, Moon{
			CalendarID: builderDraftID, Name: m.Name,
			CycleDays: m.Period, PhaseOffset: m.NewAt, Color: "",
		})
	}
	for _, s := range d.Seasons {
		cal.Seasons = append(cal.Seasons, Season{
			CalendarID: builderDraftID, Name: s.Name, Color: s.Color,
			StartMonth: s.StartMonth, StartDay: s.StartDay,
			EndMonth: s.EndMonth, EndDay: s.EndDay,
		})
	}
	for i, e := range d.Eras {
		cal.Eras = append(cal.Eras, Era{
			CalendarID: builderDraftID, Name: e.Name, StartYear: e.StartYear,
			Color: e.Color, SortOrder: i,
		})
	}
	return cal
}

// builderPreviewBlock projects the draft through the SHIPPED producer.
//
// Events is nil and that is correct AND safe, not a stub: projectBlock runs
// filterEventsByUser(nil, …) → empty and nils in.Events immediately after. Zero
// marks, foot total 0. A calendar that does not exist has no events, and the
// Block says so honestly rather than drawing a fabricated month.
//
// LayerPrefs is the zero value, which resolves to DEF — ["moons"], "a month
// with its moon phases and nothing else" — with HasSwitchboard false and
// PersistURL "" ([WZ-2c] SIGNED; the 2026-07-28 DEF/zone-chip ruling §1 says
// choosing the layer set is a HOST decision, and the wizard is a host).
//
// monthIndex is the PAGER's cursor and is clamped to the month list here rather
// than rejected: it is a UI position, not authored data, and a structure edit
// that deletes the month you were looking at must not 400 the preview.
func builderPreviewBlock(d *builderDraft, monthIndex int) calblock.BlockData {
	cal := draftCalendar(d)
	if monthIndex < 0 || monthIndex >= len(cal.Months) {
		monthIndex = 0
	}
	return projectBlock(BlockProjectionInput{
		Calendar:   cal,
		Events:     nil,
		Viewer:     BlockViewer{Role: builderViewerRole},
		MonthIndex: monthIndex,
		Year:       cal.CurrentYear,
		MoonCap:    builderMoonCap,
		// The wizard is a HOST and docks neither zone: at the drawn geometry
		// the preview column resolves to TierStd, where the Ledger does not
		// render anyway — correct for a preview, because a calendar with no
		// events has nothing to answer with.
		LedgerHidden: false,
		ShelfHidden:  true,
	})
}

// builderMoonCap matches the Bench's: the grid draws at most three bodies and
// NAMES the rest rather than growing. SKY_MAX in the mockup is the same 3.
const builderMoonCap = 3

// builderViewerRole is the ONLY role that ever sees this surface. Every route
// in §7.1 carries campaigns.RequireRole(RoleOwner), so the viewer of a preview
// is by construction an owner — see builder_handler.go for why that is written
// down rather than assumed.
const builderViewerRole = 100

// ── derived facts the stations print ────────────────────────────────────────

// builderMonthDays is the sum of the non-intercalary months.
func builderMonthDays(d *builderDraft) int {
	total := 0
	for _, m := range d.Months {
		if !m.Intercalary && m.Days > 0 {
			total += m.Days
		}
	}
	return total
}

// builderIntercalaryDays is the count of festival days outside every week.
func builderIntercalaryDays(d *builderDraft) int {
	n := 0
	for _, m := range d.Months {
		if m.Intercalary {
			n++
		}
	}
	return n
}

// builderYearDays is the whole year: months plus the festival days between
// them. It is the number the Structure station's running sum proves.
func builderYearDays(d *builderDraft) int {
	return builderMonthDays(d) + builderIntercalaryDays(d)
}

// builderBrokenMonth returns the first month that declares no days, or nil.
//
// IT IS THE WIZARD'S CENTRAL FAULT and it is real on main in both directions:
// blockDateLine already synthesises the structural fault family, and both
// ApplyImport and SetMonths reject Days <= 0 server-side. The honesty state is
// enforced by the model, not offered by the client.
func builderBrokenMonth(d *builderDraft) *builderMonth {
	for i := range d.Months {
		if !d.Months[i].Intercalary && d.Months[i].Days < 1 {
			return &d.Months[i]
		}
	}
	return nil
}

// builderNextLeaps prints the next N leap years so the rule proves itself.
// Nothing is authored year by year — the list is derived from the modulus.
func builderNextLeaps(d *builderDraft, n int) []int {
	if d.LeapEvery < 1 || d.LeapAdd < 1 {
		return nil
	}
	out := make([]int, 0, n)
	y := ((d.Year / d.LeapEvery) + 1) * d.LeapEvery
	for len(out) < n {
		out = append(out, y)
		y += d.LeapEvery
	}
	return out
}

// builderWeekSplit is the half-mark column, generalised from the signed "week
// of ten" case: at even lengths of eight or more the grid splits the week at
// its midpoint with a stronger rule; odd and short weeks get no split.
//
// A LITERAL 7 IS LEGAL AS PRESET DATA AND ILLEGAL AS LAYOUT ([WZ-11a] SIGNED).
// Gregorian genuinely has a seven-day week and seven weekday names; nothing in
// this function, in the geometry or in the grid knows the number seven.
func builderWeekSplit(week int) int {
	if week >= 8 && week%2 == 0 {
		return week / 2
	}
	return 0
}

// ── the create path ─────────────────────────────────────────────────────────

// builderImportResult turns a validated draft into the SAME *ImportResult the
// four parsers produce, so Create runs the SAME ApplyImport the importer runs.
//
// THIS IS WHAT COLLAPSES TWO FEATURES INTO ONE ([WZ-12] / dispatch §2.2). There
// is no preset table, no preset migration, no new parser and no second apply
// path: the preset gallery and the importer are one code path with two front
// doors, which is precisely what L6 ("wraps, does not replace") asks for — and
// it makes every preset round-trip-provable against export.go's BuildExport.
func builderImportResult(d *builderDraft) *ImportResult {
	res := &ImportResult{
		Format:       FormatChronicle,
		CalendarName: d.Name,
		Settings: ImportedSettings{
			CurrentYear:      d.Year,
			HoursPerDay:      24,
			MinutesPerHour:   60,
			SecondsPerMinute: 60,
			LeapYearEvery:    d.LeapEvery,
		},
	}
	if epoch := strings.TrimSpace(d.EpochName); epoch != "" {
		res.Settings.EpochName = &epoch
	}
	for i, m := range d.Months {
		res.Months = append(res.Months, MonthInput{
			Name: m.Name, Days: m.Days, SortOrder: i,
			IsIntercalary: m.Intercalary, LeapYearDays: m.LeapDays,
		})
	}
	for i, w := range d.Weekdays {
		res.Weekdays = append(res.Weekdays, WeekdayInput{Name: w, SortOrder: i})
	}
	for _, m := range d.Moons {
		res.Moons = append(res.Moons, MoonInput{
			Name: m.Name, CycleDays: m.Period, PhaseOffset: m.NewAt, Color: "#c0c0c0",
		})
	}
	for _, s := range d.Seasons {
		res.Seasons = append(res.Seasons, Season{
			Name: s.Name, Color: s.Color,
			StartMonth: s.StartMonth, StartDay: s.StartDay,
			EndMonth: s.EndMonth, EndDay: s.EndDay,
		})
	}
	for i, e := range d.Eras {
		res.Eras = append(res.Eras, EraInput{
			Name: e.Name, StartYear: e.StartYear, Color: e.Color, SortOrder: i,
		})
	}
	return res
}

// builderCreateBlocked returns the reason Create is held, or "".
//
// TWO GATES, AND THEY ARE DIFFERENT KINDS OF THING.
//
// The first is STRUCTURAL and is enforced everywhere: a month with no days
// cannot resolve a date, blockDateLine says so where the date would go, and
// both ApplyImport and SetMonths refuse it.
//
// The second is WIZARD-LOCAL POLICY ([WZ-3] SIGNED), and it must not be
// mistaken for a claim about the model. Chronicle has NO era-relative year
// numbering: a calendar with zero eras still resolves "Deepwinter 14, 1523"
// perfectly well, and block_projection.go's own comment records that wave 1
// examined the era fault and explicitly REFUSED to synthesise it, flagging the
// question to the coordinator. THAT FLAG IS CLOSED HERE, and it is closed as
// policy rather than as model: the wizard may decline to CREATE what would read
// ambiguously, even where the Block would render it. That is a statement about
// what this authoring surface accepts, and the copy says so in those words —
// "this calendar has no era", never "a year cannot resolve", which would be
// false.
func builderCreateBlocked(d *builderDraft) string {
	if bad := builderBrokenMonth(d); bad != nil {
		name := bad.Name
		if name == "" {
			name = "a month"
		}
		return fmt.Sprintf("%s declares 0 days — give it days in Structure, or remove it", name)
	}
	if len(d.Months) == 0 {
		return "no months are declared — a year needs at least one"
	}
	if len(d.Eras) == 0 {
		return "this calendar has no era — the wizard holds Create until one is added"
	}
	return ""
}
