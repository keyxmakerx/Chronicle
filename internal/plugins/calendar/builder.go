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
	"strconv"
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

// ═══════════════════════════════════════════════════════════════════════════
// THE VIEW MODEL
//
// Everything below is what builder.templ consumes. It is built in the handler
// and the template decides nothing — the wizard's honesty states are FACTS
// computed here, so a template branch can never be the only thing standing
// between a player and a chip, or between an author and a fault.
// ═══════════════════════════════════════════════════════════════════════════

// BuilderViewData is one render of the wizard.
type BuilderViewData struct {
	CampaignID string
	CSRFToken  string

	Draft    *builderDraft
	Step     int
	Importer bool

	// PreviewMonth is the PAGER's cursor, not the calendar's current month.
	PreviewMonth int
	Block        calblock.BlockData

	Presets []builderPreset
	// Identity is the roster entry the draft started from, or the zero value
	// for a draft the author has bent past recognition.
	Identity builderPreset

	// Carry is every field the CURRENT station does not render as a visible
	// control, emitted as hidden inputs so the whole declaration round-trips
	// through the form. It is what makes "no draft store" work.
	Carry []builderField

	// Blocked is the reason Create is held, or "".
	Blocked string
	// Fault is the wizard's own anchor-bar fault, drawn WHERE THE DATE WOULD
	// GO. Zero when the declaration resolves.
	Fault builderFault
	// Checks is Review's line-by-line verdict.
	Checks []builderCheck
	// Stats is the preview column's declaration readout.
	Stats []builderStat

	// Importer state — the detected file's name and its mapping table. Empty
	// until something is dropped.
	Detected string
	Mapping  []builderMapRow
}

// builderField is one hidden input.
type builderField struct{ Name, Value string }

// builderFault is the honesty state drawn where the date would go: a warn rail,
// a warn headline, a plain-language cause and a jump to the station that fixes
// it. NEVER a zero, never a dash, never a placeholder.
type builderFault struct {
	Headline string
	Why      string
	FixStep  string
	FixLabel string
}

// builderCheck is one Review line. Kind is ok | warn | need, and `need` is the
// SIGNED needs-backend chip — reserved for genuine backend gaps and never used
// as decoration.
type builderCheck struct {
	Kind string
	Text string
}

// builderStat is one row of the preview column's declaration readout.
type builderStat struct {
	Label string
	Value string
	// Warn puts the value in warn ink. The Year-length stat uses it while a
	// month declares 0 days, because an unresolvable number must not print as
	// a confident one.
	Warn bool
	// Need hangs the signed needs-backend chip on the row. Exactly one stat
	// can carry it — the leap exception clause Chronicle cannot store.
	Need string
}

// builderMapRow is one line of the importer's mapping table, which the mockup
// calls its own honesty mechanism: every fact in the file, the station it
// lands on, and what happened to it.
type builderMapRow struct {
	Fact    string
	Station string
	Kind    string // ok | warn | fact
	Verdict string
}

// builderStationOwns reports whether the station at `step` renders this field
// family as a VISIBLE control. Everything it does not own is carried hidden.
//
// The split exists because there is no draft store: the whole declaration must
// survive a station change, and it survives by riding the form. A field
// rendered twice — once visibly and once hidden — would submit twice and break
// index alignment silently, which is exactly the kind of bug that produces a
// calendar with the right month count and the wrong month names.
func builderStationOwns(step int, importer bool, family string) bool {
	if importer {
		return false
	}
	switch family {
	case "month-name":
		return step == 1
	case "interc-name":
		return step == 3
	case "month-list":
		// EITHER month station emits the WHOLE ordered name list — see the
		// comment on builderCarryFields for why the split cannot be per-family.
		return step == 1 || step == 3
	case "weekday":
		return step == 2
	case "leap":
		return step == 4
	case "moon":
		return step == 5
	case "season":
		return step == 6
	case "era":
		return step == 7
	case "identity":
		return step == 8
	}
	return false
}

// builderCarryFields is every field the current station does not show.
func builderCarryFields(d *builderDraft, step int, importer bool) []builderField {
	var out []builderField
	add := func(n, v string) { out = append(out, builderField{Name: n, Value: v}) }

	// Scalars that no station edits as a list.
	add("preset", d.Preset)
	add("mode", d.Mode)
	add("hue", d.Hue)
	add("pattern", d.Pattern)
	add("letter", d.Letter)
	if d.HollowSwatch {
		add("hollow", "1")
	}
	add("leap_note", d.LeapNote)
	if !builderStationOwns(step, importer, "identity") {
		add("cal_name", d.Name)
		add("epoch", d.EpochName)
		add("year", strconv.Itoa(d.Year))
		add("tz", d.TimeZone)
	}
	// The leap MODULUS is changed by a stepper act, never typed, so it always
	// rides hidden — exactly like a month's day count. Only the two authored
	// strings move with the station that owns them.
	add("leap_every", strconv.Itoa(d.LeapEvery))
	add("leap_add", strconv.Itoa(d.LeapAdd))
	if !builderStationOwns(step, importer, "leap") {
		add("leap_name", d.LeapName)
		add("leap_after", d.LeapAfter)
	}

	// Months. Days, the intercalary flag and the leap-day count always ride
	// hidden — they are changed by a stepper act, never typed — and only the
	// NAME moves between visible and hidden with the station that owns it.
	for _, m := range d.Months {
		add("m_days", strconv.Itoa(m.Days))
		add("m_leapdays", strconv.Itoa(m.LeapDays))
		if m.Intercalary {
			add("m_inter", "1")
		} else {
			add("m_inter", "0")
		}
	}
	// THE NAME LIST IS EMITTED WHOLE OR NOT AT ALL, AND THAT IS A BUG FIX WITH
	// A TEST BEHIND IT. Splitting it per-family — visible names from the owning
	// station, hidden names from here — concatenates two groups in submission
	// order rather than interleaving them, so `m_name[i]` stops lining up with
	// `m_days[i]` and every month silently takes its neighbour's name. The
	// round-trip test caught exactly that. Either month station therefore emits
	// the WHOLE ordered list itself (visible for its own family, hidden in place
	// for the other), and the carry emits it only when neither station is open.
	if !builderStationOwns(step, importer, "month-list") {
		for _, m := range d.Months {
			add("m_name", m.Name)
		}
	}

	if !builderStationOwns(step, importer, "weekday") {
		for _, w := range d.Weekdays {
			add("wd", w)
		}
	}
	if !builderStationOwns(step, importer, "moon") {
		for _, m := range d.Moons {
			add("moon_name", m.Name)
			add("moon_period", strconv.FormatFloat(m.Period, 'f', -1, 64))
			add("moon_newat", strconv.FormatFloat(m.NewAt, 'f', -1, 64))
		}
	}
	for _, s := range d.Seasons {
		// A season's colour is AUTHORED DATA and no station offers a picker in
		// this wave, so it always rides hidden — as do its bounds, which come
		// from the format that declared them.
		add("season_color", s.Color)
		add("season_cname", s.ColorName)
		add("season_sm", strconv.Itoa(s.StartMonth))
		add("season_sd", strconv.Itoa(s.StartDay))
		add("season_em", strconv.Itoa(s.EndMonth))
		add("season_ed", strconv.Itoa(s.EndDay))
		if !builderStationOwns(step, importer, "season") {
			add("season_name", s.Name)
		}
	}
	for _, e := range d.Eras {
		add("era_color", e.Color)
		add("era_cname", e.ColorName)
		if !builderStationOwns(step, importer, "era") {
			add("era_name", e.Name)
			add("era_code", e.Code)
			add("era_year", strconv.Itoa(e.StartYear))
		}
	}
	return out
}

// builderMonthIndexes returns the positions of the months a station owns, so
// the template can walk the ONE ordered list without re-deriving the split.
func builderMonthIndexes(d *builderDraft, intercalary bool) []int {
	var out []int
	for i, m := range d.Months {
		if m.Intercalary == intercalary {
			out = append(out, i)
		}
	}
	return out
}

// builderPrecedingMonth names the month an intercalary day follows. The mockup
// authors "{name} after {month}"; Chronicle stores the order and nothing else,
// so the answer is read off the list rather than carried beside it.
func builderPrecedingMonth(d *builderDraft, idx int) string {
	for i := idx - 1; i >= 0; i-- {
		if !d.Months[i].Intercalary {
			return d.Months[i].Name
		}
	}
	return "the start of the year"
}

// builderFaultFor is the wizard's own anchor-bar fault.
//
// IT IS NOT A SECOND FAULT SYSTEM. blockDateLine's structural family renders
// inside the Block, unmodified, wherever the CURRENT date cannot resolve. This
// one answers a different question — "is anything in the declaration broken",
// which is true even when month 1 is fine and month 3 is not — and it is the
// wizard's chrome, above the Block, with a jump to the station that fixes it.
func builderFaultFor(d *builderDraft) builderFault {
	if bad := builderBrokenMonth(d); bad != nil {
		name := bad.Name
		if name == "" {
			name = "an unnamed month"
		}
		return builderFault{
			Headline: "Cannot resolve a date",
			Why: fmt.Sprintf("%s declares 0 days, so the year cannot be walked past it. "+
				"Give it days in Structure, or remove it.", name),
			FixStep: "structure", FixLabel: "Fix in Structure",
		}
	}
	if len(d.Months) == 0 {
		return builderFault{
			Headline: "Cannot resolve a date",
			Why:      "No months are declared, so there is no year to walk. Add one in Structure.",
			FixStep:  "structure", FixLabel: "Fix in Structure",
		}
	}
	if len(d.Eras) == 0 {
		// [WZ-3] SIGNED, and the copy is the ruling. The headline is a claim
		// about THE WIZARD, never about the model: Chronicle has no
		// era-relative year numbering and a zero-era calendar resolves its year
		// perfectly well, so "cannot resolve a year" would be false. The wizard
		// may decline to CREATE what would read ambiguously; it may not
		// misdescribe why.
		return builderFault{
			Headline: "This calendar has no era — Create waits",
			Why: "Nothing is wrong with the dates: they resolve. The wizard holds " +
				"Create until an era exists so the year has a name to read. Add one in Eras.",
			FixStep: "eras", FixLabel: "Fix in Eras",
		}
	}
	return builderFault{}
}

// ── paths and small readings the template needs ─────────────────────────────
//
// THE PATHS ARE BUILT HERE, NOT IN THE TEMPLATE. A route string assembled in
// markup is a route string no test can find.

func builderPagePath(campaignID string) string {
	return fmt.Sprintf("/campaigns/%s/calendars/builder", campaignID)
}

func builderPreviewPath(campaignID string) string {
	return fmt.Sprintf("/campaigns/%s/calendars/builder/preview", campaignID)
}

// builderStepPath is a station's own URL, so a station is bookmarkable and a
// refresh mid-wizard lands where the author was.
func builderStepPath(campaignID, step string) string {
	return fmt.Sprintf("%s?step=%s", builderPagePath(campaignID), step)
}

// builderStepLabel is the wtop's readout: "Start" at 0, "Step N of 8" after.
func builderStepLabel(step int, importer bool) string {
	if importer {
		return "Importer"
	}
	if step <= 0 {
		return "Start"
	}
	return fmt.Sprintf("Step %d of %d · %s", step, len(builderStations)-1, builderStations[step].Name)
}

// builderRailState is cur | done | todo.
func builderRailState(i, step int) string {
	switch {
	case i == step:
		return "cur"
	case i < step:
		return "done"
	default:
		return "todo"
	}
}

// builderRailMark is the number box's content: a tick for a station already
// walked, the index otherwise.
func builderRailMark(i, step int) string {
	if i < step {
		return "✓"
	}
	return strconv.Itoa(i)
}

// builderLadder is the --m-i inline custom property that drives the step
// panel's delay ladder. It is the ONLY place a row carries an animation
// position, and the guard asserts the ladder appears in no other prelude.
func builderLadder(i int) string { return fmt.Sprintf("--m-i:%d", i) }

// builderAxis sets the --axis channel from a CLOSED calendar identity token.
// An empty hue is Blank's UNCHOSEN identity and falls back to a neutral RULE
// token — never to --own-none, which is the owner axis and means something
// else entirely.
func builderAxis(hue string) string {
	if strings.TrimSpace(hue) == "" {
		return "--axis:var(--rule-structural-strong)"
	}
	return fmt.Sprintf("--axis:var(--cal-%s)", hue)
}

// builderSwatchStyle sets a swatch to an AUTHORED colour (Season.Color /
// Era.Color), which is its own value and never a borrowed axis hue.
func builderSwatchStyle(colour string) string {
	if strings.TrimSpace(colour) == "" {
		return "--axis:var(--rule-structural)"
	}
	return fmt.Sprintf("--axis:%s", colour)
}

// builderNum formats a moon's period or offset without a trailing zero storm.
func builderNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// builderMonthSummary is the Structure station's running sum, and it is the
// arithmetic the year is made of rather than a label for it.
func builderMonthSummary(d *builderDraft) string {
	months := len(builderMonthIndexes(d, false))
	if ic := builderIntercalaryDays(d); ic > 0 {
		return fmt.Sprintf("%d months · %d month days · +%d intercalary = %d days a year",
			months, builderMonthDays(d), ic, builderYearDays(d))
	}
	return fmt.Sprintf("%d months · %d month days = %d days a year",
		months, builderMonthDays(d), builderYearDays(d))
}

// builderWeekSummary proves the week's arithmetic against a real month.
func builderWeekSummary(d *builderDraft) string {
	week := len(d.Weekdays)
	if week < 1 || len(d.Months) == 0 {
		return ""
	}
	for _, m := range d.Months {
		if m.Intercalary || m.Days < 1 {
			continue
		}
		return fmt.Sprintf("A %d-day month is %d rows of %d.", m.Days, (m.Days+week-1)/week, week)
	}
	return ""
}

// builderSplitNote states the half-mark rule and where it lands, or that no
// split is drawn at this length.
func builderSplitNote(d *builderDraft) string {
	week := len(d.Weekdays)
	half := builderWeekSplit(week)
	if half == 0 {
		return "— at this length no split is drawn"
	}
	name := ""
	if half-1 < len(d.Weekdays) {
		name = d.Weekdays[half-1]
	}
	return fmt.Sprintf("— here after %s (%d+%d)", name, half, half)
}

// builderMoonTurns reads the turn marks a moon makes in the previewed month.
// Nothing is authored date by date: the marks come from the SAME Block data
// the grid drew, so the station and the month cannot disagree.
func builderMoonTurns(data BuilderViewData, moonIndex int) []string {
	var out []string
	for _, m := range data.Block.Month.Almanac {
		if m.Name == "" || moonIndex >= len(data.Draft.Moons) ||
			m.Name != data.Draft.Moons[moonIndex].Name {
			continue
		}
		for _, day := range m.Days {
			if day.Turn != "" {
				out = append(out, day.Turn)
			}
		}
	}
	return out
}

// builderSeasonSpan reads a season's bounds back as a sentence.
func builderSeasonSpan(d *builderDraft, s builderSeason) string {
	name := func(month int) string {
		idx, seen := 0, 0
		for i, m := range d.Months {
			if m.Intercalary {
				continue
			}
			seen++
			if seen == month {
				idx = i
				return d.Months[idx].Name
			}
		}
		return fmt.Sprintf("month %d", month)
	}
	if s.StartMonth == 0 && s.EndMonth == 0 {
		return "no span declared"
	}
	return fmt.Sprintf("%s to %s", name(s.StartMonth), name(s.EndMonth))
}

// builderActValue encodes one panel action. The verb, an optional row index and
// a direction, in one form value, because a button submits one name/value pair.
func builderActValue(act string, index, dir int) string {
	if index >= 0 {
		return fmt.Sprintf("%s:%d:%d", act, index, dir)
	}
	return fmt.Sprintf("%s::%d", act, dir)
}

// builderCountLabel reads a count, or says "none" rather than printing a zero.
func builderCountLabel(n int) string {
	if n == 0 {
		return "none"
	}
	return strconv.Itoa(n)
}

// builderCheckLabel is the badge text for a Review verdict. `need` reads
// "needs backend" in full — the SIGNED chip, never an abbreviation of it.
func builderCheckLabel(kind string) string {
	switch kind {
	case "ok":
		return "ok"
	case "need":
		return "needs backend"
	default:
		return "fix"
	}
}

// builderPresetMeta is a preset card's fact line.
func builderPresetMeta(p builderPreset) string {
	d, err := builderPresetDraft(p.Key)
	if err != nil {
		return ""
	}
	moons := "no moons"
	if n := len(d.Moons); n == 1 {
		moons = "1 moon"
	} else if n > 1 {
		moons = fmt.Sprintf("%d moons", n)
	}
	return fmt.Sprintf("%d months · %d days · %d-day week · %s",
		len(builderMonthIndexes(d, false)), builderYearDays(d), len(d.Weekdays), moons)
}

// builderPreviewSub is the preview column's one-line reading of the whole
// declaration. It says "year incomplete" rather than printing a total that a
// 0-day month makes meaningless.
func builderPreviewSub(d *builderDraft) string {
	months := len(builderMonthIndexes(d, false))
	year := fmt.Sprintf("%d days a year", builderYearDays(d))
	if builderBrokenMonth(d) != nil {
		year = "year incomplete"
	}
	out := fmt.Sprintf("%s · %s · %d-day week", builderPlural(months, "month"), year, len(d.Weekdays))
	if n := len(d.Moons); n > builderMoonCap {
		out += fmt.Sprintf(" · %d of %d moons drawn", builderMoonCap, n)
	} else if n > 0 {
		out += " · " + builderPlural(n, "moon")
	}
	return out
}

// builderPlural is "1 month" / "2 months".
func builderPlural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// builderMonthLenLabel reads the previewed month's length, in WARN ink when it
// is zero — the pager must never print a confident "0 days".
func builderMonthLenLabel(data BuilderViewData) string {
	if data.PreviewMonth < 0 || data.PreviewMonth >= len(data.Draft.Months) {
		return ""
	}
	if d := data.Draft.Months[data.PreviewMonth]; d.Days < 1 {
		return "0 days"
	}
	return fmt.Sprintf("%d days", data.Draft.Months[data.PreviewMonth].Days)
}

// builderPagerLabel is "3 of 12".
func builderPagerLabel(data BuilderViewData) string {
	if len(data.Draft.Months) == 0 {
		return ""
	}
	return fmt.Sprintf("%d of %d", data.PreviewMonth+1, len(data.Draft.Months))
}

// builderStatsFor is the preview column's declaration readout.
func builderStatsFor(d *builderDraft) []builderStat {
	bad := builderBrokenMonth(d)

	leap := "none"
	if d.LeapEvery > 0 {
		leap = fmt.Sprintf("+%d every %d years", d.LeapAdd, d.LeapEvery)
	}
	moons := "none declared — the sky layer stays absent"
	if n := len(d.Moons); n > builderMoonCap {
		moons = fmt.Sprintf("%d drawn · %d almanac-only", builderMoonCap, n-builderMoonCap)
	} else if n > 0 {
		moons = fmt.Sprintf("%d drawn", n)
	}
	seasons := "none"
	if len(d.Seasons) > 0 {
		names := make([]string, 0, len(d.Seasons))
		for _, s := range d.Seasons {
			names = append(names, s.Name)
		}
		seasons = strings.Join(names, " · ")
	}
	eras := "none yet"
	if len(d.Eras) > 0 {
		codes := make([]string, 0, len(d.Eras))
		for _, e := range d.Eras {
			if e.Code != "" {
				codes = append(codes, e.Code)
			} else {
				codes = append(codes, e.Name)
			}
		}
		eras = strings.Join(codes, " · ")
	}

	yearLen := fmt.Sprintf("%d days", builderYearDays(d))
	if d.LeapEvery > 0 && d.LeapAdd > 0 {
		yearLen += fmt.Sprintf(" · %d on leap years", builderYearDays(d)+d.LeapAdd)
	}
	yearWarn := false
	if bad != nil {
		name := bad.Name
		if name == "" {
			name = "a month"
		}
		yearLen = fmt.Sprintf("unresolvable while %s has 0 days", name)
		yearWarn = true
	}

	stats := []builderStat{
		{Label: "Months", Value: fmt.Sprintf("%d · %d days",
			len(builderMonthIndexes(d, false)), builderMonthDays(d))},
		{Label: "Week", Value: builderWeekStat(d)},
		{Label: "Intercalary", Value: builderIntercalaryStat(d)},
		{Label: "Leap", Value: leap, Need: d.LeapNote},
		{Label: "Moons", Value: moons},
		{Label: "Seasons", Value: seasons},
		{Label: "Eras", Value: eras},
		{Label: "Year length", Value: yearLen, Warn: yearWarn},
	}
	if d.LeapNote != "" {
		stats[3].Value = leap + " · " + d.LeapNote
	}
	return stats
}

func builderWeekStat(d *builderDraft) string {
	week := len(d.Weekdays)
	if half := builderWeekSplit(week); half > 0 {
		return fmt.Sprintf("%d days · splits %d+%d", week, half, half)
	}
	return fmt.Sprintf("%d days", week)
}

func builderIntercalaryStat(d *builderDraft) string {
	if n := builderIntercalaryDays(d); n > 0 {
		return builderPlural(n, "festival day")
	}
	return "none"
}

// builderChecksFor is Review's line-by-line verdict.
//
// `need` IS THE SIGNED needs-backend CHIP AND IT APPEARS ONLY WHERE THE TEXT IS
// LITERALLY ABOUT A BACKEND GAP. LAYERS-P9 §10 states the rule: add no
// `.badge need` use that is not literally "needs backend". Everything else that
// wants the author's hand takes `warn`, and every neutral state readout takes
// `.wz-fact` — the class this surface mints precisely so `.need` is not diluted
// into a fourth grey chip meaning a third thing.
func builderChecksFor(d *builderDraft) []builderCheck {
	var out []builderCheck

	if bad := builderBrokenMonth(d); bad != nil {
		name := bad.Name
		if name == "" {
			name = "an unnamed month"
		}
		out = append(out, builderCheck{Kind: "warn",
			Text: fmt.Sprintf("Dates do not resolve — %s declares 0 days", name)})
	} else if len(d.Months) == 0 {
		out = append(out, builderCheck{Kind: "warn", Text: "No months are declared"})
	} else {
		last := d.Months[len(d.Months)-1]
		out = append(out, builderCheck{Kind: "ok", Text: fmt.Sprintf(
			"Every date resolves — the year walks 1 %s to %d %s with no gaps",
			d.Months[0].Name, last.Days, last.Name)})
	}

	seen := map[string]bool{}
	dupes := false
	for _, w := range d.Weekdays {
		if seen[w] {
			dupes = true
		}
		seen[w] = true
	}
	if dupes {
		out = append(out, builderCheck{Kind: "warn", Text: "Weekday names repeat"})
	} else {
		out = append(out, builderCheck{Kind: "ok", Text: fmt.Sprintf(
			"Weekday names unique (%d of %d)", len(seen), len(d.Weekdays))})
	}

	if d.LeapEvery > 0 && d.LeapAdd > 0 {
		out = append(out, builderCheck{Kind: "ok", Text: fmt.Sprintf(
			"Year length stable: %d, %d on leap years",
			builderYearDays(d), builderYearDays(d)+d.LeapAdd)})
	} else {
		out = append(out, builderCheck{Kind: "ok", Text: fmt.Sprintf(
			"Year length fixed: %d days", builderYearDays(d))})
	}

	// AN EXCEPTION CHRONICLE CANNOT STORE IS NEVER CERTIFIED GREEN.
	if d.LeapNote != "" {
		out = append(out, builderCheck{Kind: "need", Text: fmt.Sprintf(
			"The exception %q has no home in Chronicle's single-modulus leap model — "+
				"Create writes +%d every %d and drops the clause", d.LeapNote, d.LeapAdd, d.LeapEvery)})
	}

	switch n := len(d.Moons); {
	case n > builderMoonCap:
		var extra []string
		for _, m := range d.Moons[builderMoonCap:] {
			extra = append(extra, m.Name)
		}
		out = append(out, builderCheck{Kind: "ok", Text: fmt.Sprintf(
			"Moons: %d drawn in the grid, %d almanac-only (%s)",
			builderMoonCap, n-builderMoonCap, strings.Join(extra, ", "))})
	case n > 0:
		out = append(out, builderCheck{Kind: "ok", Text: fmt.Sprintf(
			"Moons: %d, all drawn in the grid", n)})
	default:
		out = append(out, builderCheck{Kind: "ok", Text: "No moons — the sky layer stays absent"})
	}

	if len(d.Eras) > 0 {
		reading := d.EpochName
		if reading == "" {
			reading = "the year alone"
		}
		out = append(out, builderCheck{Kind: "ok", Text: fmt.Sprintf(
			"Era present — years read %q", fmt.Sprintf("%s %d", reading, d.Year))})
	} else {
		// [WZ-3] SIGNED: a claim about the WIZARD, never about the model.
		out = append(out, builderCheck{Kind: "warn",
			Text: "0 eras — this calendar has no era, and the wizard holds Create until one exists"})
	}

	if d.Mode == ModeRealLife {
		if strings.TrimSpace(d.TimeZone) == "" {
			out = append(out, builderCheck{Kind: "warn",
				Text: "A real-world calendar needs an IANA timezone — its date comes from the wall clock in that zone"})
		} else {
			out = append(out, builderCheck{Kind: "ok", Text: fmt.Sprintf(
				"Wall-clock authoritative in %s — the date is read from the real world and cannot be advanced by hand",
				d.TimeZone)})
		}
	}
	return out
}

// builderMappingFor is the importer's mapping table: every fact in the file,
// the station it lands on, and what happened to it.
//
// THE FILE CALLS THIS ITS OWN HONESTY MECHANISM. A row that says "left empty"
// is the difference between an import that silently dropped something and one
// that told you.
func builderMappingFor(res *ImportResult, d *builderDraft) []builderMapRow {
	rows := []builderMapRow{
		{Fact: fmt.Sprintf("%d months, %d days", len(builderMonthIndexes(d, false)), builderMonthDays(d)),
			Station: "Structure", Kind: "ok", Verdict: "mapped"},
		{Fact: fmt.Sprintf("%d weekday names", len(d.Weekdays)),
			Station: "Week", Kind: "ok", Verdict: "mapped"},
	}
	if n := builderIntercalaryDays(d); n > 0 {
		rows = append(rows, builderMapRow{
			Fact:    fmt.Sprintf("%d festival days outside weeks", n),
			Station: "Intercalary", Kind: "ok", Verdict: "mapped"})
	} else {
		rows = append(rows, builderMapRow{Fact: "no festival days outside weeks",
			Station: "Intercalary", Kind: "fact", Verdict: "left empty"})
	}
	if d.LeapEvery > 0 {
		rows = append(rows, builderMapRow{
			Fact:    fmt.Sprintf("leap: +%d every %d years", d.LeapAdd, d.LeapEvery),
			Station: "Leap rules", Kind: "ok", Verdict: "mapped"})
	} else {
		rows = append(rows, builderMapRow{Fact: "no leap rule declared",
			Station: "Leap rules", Kind: "fact", Verdict: "left empty"})
	}
	if len(d.Moons) > 0 {
		rows = append(rows, builderMapRow{
			Fact:    fmt.Sprintf("%d moons with periods", len(d.Moons)),
			Station: "Moons", Kind: "ok", Verdict: "mapped"})
	} else {
		rows = append(rows, builderMapRow{Fact: "no moons carried by this file",
			Station: "Moons", Kind: "fact", Verdict: "left empty"})
	}
	if len(d.Seasons) > 0 {
		rows = append(rows, builderMapRow{
			Fact:    fmt.Sprintf("%d seasons with bounds", len(d.Seasons)),
			Station: "Seasons", Kind: "ok", Verdict: "mapped"})
	} else {
		rows = append(rows, builderMapRow{Fact: "seasons not carried by this format",
			Station: "Seasons", Kind: "fact", Verdict: "left empty"})
	}
	// SIMPLE CALENDAR GENUINELY CARRIES NO ERAS — parseSimpleCalendarInner never
	// populates result.Eras; only the Calendaria, Fantasy-Cal and Chronicle
	// parsers do. So this row is a real outcome of a real file rather than a
	// demonstration, and it is the row that makes the wizard's era gate visible
	// at the moment it starts mattering.
	if len(d.Eras) > 0 {
		rows = append(rows, builderMapRow{
			Fact:    fmt.Sprintf("%d eras", len(d.Eras)),
			Station: "Eras", Kind: "ok", Verdict: "mapped"})
	} else {
		rows = append(rows, builderMapRow{
			Fact:    "eras not carried by this format — the wizard holds Create until one exists",
			Station: "Eras", Kind: "warn", Verdict: "add before Create"})
	}
	if res != nil && res.Format != "" {
		rows = append(rows, builderMapRow{
			Fact:    fmt.Sprintf("detected as %s", builderFormatLabel(res.Format)),
			Station: "Start", Kind: "ok", Verdict: "detected"})
	}
	return rows
}

// builderFormatLabel names a format the way its own product names it. THE FOUR
// ARE THE FOUR ON MAIN and there is no fifth: an earlier revision of the mockup
// invented two, and the design review failed it for exactly that.
func builderFormatLabel(f ImportFormat) string {
	switch f {
	case FormatChronicle:
		return "Chronicle native"
	case FormatSimpleCal:
		return "Simple Calendar"
	case FormatCalendaria:
		return "Calendaria"
	case FormatFantasyCal:
		return "Fantasy-Calendar.com"
	}
	return string(f)
}

// builderFormats is the roster the importer's four cards read from.
var builderFormats = []struct{ Name, Detail string }{
	{"Simple Calendar", "Foundry module export · months, weekdays, leap, moons"},
	{"Calendaria", "Foundry module export · months, weekdays, moons, eras"},
	{"Fantasy-Calendar.com", "JSON export · full structure, eras, seasons"},
	{"Chronicle native", "chronicle-calendar-v1 · a calendar exported from Chronicle"},
}

// builderRowNumber is a month's ordinal WITHIN ITS OWN FAMILY — the number the
// Structure and Intercalary stations print in their index column. The
// underlying list is one ordered slice, so the two stations count separately
// while sharing it.
func builderRowNumber(d *builderDraft, idx int) int {
	if idx < 0 || idx >= len(d.Months) {
		return 0
	}
	want, n := d.Months[idx].Intercalary, 0
	for i := 0; i <= idx; i++ {
		if d.Months[i].Intercalary == want {
			n++
		}
	}
	return n
}
