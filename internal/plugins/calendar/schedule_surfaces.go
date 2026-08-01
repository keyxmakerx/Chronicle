// schedule_surfaces.go — the five surfaces' view models (C-CALV4-RSVP-P8
// Part B, S1–S5) and the builders that fill them.
//
// THE EMPTY STATES ARE SIGNED TOO. Every builder below has a path for a
// campaign where nothing has been entered, and none of them makes the
// instrument vanish: no fabricated card, no zeros, no em-dashes standing in for
// data. A surface that disappears when it has nothing to say teaches the reader
// that it is decoration.
package calendar

// scheduleAvailPreferred is the sessions plugin's `preferred` lane state, as a
// LOCAL constant.
//
// It is a literal rather than an import on purpose:
// internal/wire/plugin_import_guard_test.go forbids a compile-time edge from
// calendar to sessions outright, and the whole BenchScheduleReader seam exists
// to keep that true. The value is the wire contract's own string and it crosses
// this seam in BenchLaneSegment.State, so a change on the sessions side is a
// change to the seam's contract and is caught where seams are caught.
const scheduleAvailPreferred = "preferred"

// scheduleAvailAvailable is the same for the plain-available lane state.
const scheduleAvailAvailable = "available"

// AvailPreferred / AvailAvailable are the names the builders read. Aliased so
// the arithmetic in schedule_sections.go reads like the domain rather than like
// a string compare.
const (
	AvailPreferred = scheduleAvailPreferred
	AvailAvailable = scheduleAvailAvailable
)

// --- the captions -----------------------------------------------------------

// ScheduleCaption is one caption paragraph, carried as RUNS so the sealed
// drawing's emphasis survives the trip from copy to page.
//
// THE BOLD LEAD-IN IS NOT DECORATION. The mockup writes every caption with one —
// `<b>How it ranks:</b>`, `<b>What the score cannot include:</b>`,
// `<b>Fine and coarse disagree, on purpose:</b>` — and it is the mechanism by
// which a reader FINDS a named honesty claim inside a paragraph of grey prose.
// Two of this surface's own honesty claims ARE lead-ins; shipping them flat
// leaves the page saying the same words and admitting nothing findable.
//
// RUNS, NEVER A MARKUP STRING. templ escapes text and must go on escaping it:
// the emphasis is a property of the copy, declared beside the words, and never a
// fragment of trusted HTML smuggled past the escaper.
type ScheduleCaption []ScheduleRun

// ScheduleRun is one span of caption prose and the emphasis drawn on it.
//
// Em is "" for plain prose, "b" for the drawn lead-in, and "i" for a vocabulary
// word the caption is QUOTING rather than using — `preferred`, `maybe`, `no`.
// The distinction matters: "an answer of in, maybe or out" is a sentence about
// three words, and the drawing italicises them so it cannot be misread as a
// sentence that is simply in.
type ScheduleRun struct {
	Text string
	Em   string
}

// Text joins the runs back into the plain sentence.
//
// Every assertion about WHAT a caption says reads this: the wording is the
// caption's contract with the arithmetic above it, and a test about wording has
// no business knowing where the eye is meant to land. Runs carry their own
// spacing, so the join is a concatenation and never inserts one.
func (c ScheduleCaption) Text() string {
	out := ""
	for _, r := range c {
		out += r.Text
	}
	return out
}

// --- S1 · THE VERDICT -------------------------------------------------------

// ScheduleVerdict is the page's lead: the ANSWER, with the evidence beneath it.
//
// THE PAGE'S ONE FILLED BUTTON LIVES HERE and is bound to the chosen window —
// the single filled control IS the answer, which is why there is no `.btn.fill`
// in the page head.
type ScheduleVerdict struct {
	Title string
	Frame string
	// Mode says what the reader is looking at, and it differs by role because
	// the DATA differs by role: a Director sees a ranking over named lanes, a
	// player sees the same ranking over tallies with no names in their payload.
	Mode  string
	Chips []scheduleChip

	// Headline is the chosen window ("Sat 25 Jul · 19:00–22:00"); HeadlineZone
	// is the muted zone sibling beside it. A real-world time gets the zone
	// sibling and NEVER a monospace face; an in-world time gets the monospace
	// face and never a zone. They can never be confused (L15).
	Headline     string
	HeadlineZone string
	HeadlineTitle string
	// Rec is the count line under the headline.
	Rec string
	// WarnLine is the honesty line: how many members show no saved
	// availability, or that everybody answered. Tone is "warn" | "good" | "".
	WarnLine string
	WarnTone string
	// MailLine is P8B's verbatim unconfigured-SMTP sentence, shared as a single
	// package constant with the endpoint that refuses the send so the surface
	// and the server cannot disagree about what it means.
	MailLine string

	// Propose is the disabled primary. It KEEPS ITS CHIP: there is still no
	// propose-from-window write path, and a page whose main action is a fiction
	// is worse than a page with no main action (ledger #19, WG-5).
	Propose *BenchAction
	// Ask is P8B's LIVE Nudge, nil for a player.
	Ask *BenchAskForm

	Candidates []ScheduleCandidate
	// Fault is the warnrow that REPLACES the candidate cards when there is
	// nothing to rank — first run, below quorum, or nothing free at all.
	Fault *ScheduleCandidateFault

	// MoreLabel / More are the lower-scoring windows, in a popover. Ordered BY
	// DATE with the score printed on the right: a list ordered by score would
	// put the ranking's own tail in a second, competing order.
	MoreLabel string
	MoreID    string
	More      []ScheduleMoreRow

	Caption ScheduleCaption
}

// ScheduleCandidate is one ranked window card.
//
// CARDS SIT IN DATE ORDER AND NEVER MOVE. Rank is a PRINTED ORDINAL, not a
// position: when somebody answers, the rank numeral re-inks and the reason
// sentence rewrites IN PLACE. A person answering may not move a single element
// on this page — that is the whole anti-motion-by-data mechanic, and it is the
// reason the ordering key here is the date and not the score.
type ScheduleCandidate struct {
	Rank     int
	Href     string
	Selected bool
	// DayKey is the ANSWER key (guard B4): every dated node on this page carries
	// data-day, in one namespace.
	DayKey string

	When string
	Zone string
	// Why is the WRITTEN REASON a window ranks where it does — the thing that
	// makes overruling the tool possible, which is the whole bet of this page.
	//
	// PERMISSION IS ABSENCE AND IT BINDS THE PROSE. A player's payload carries
	// no member, so this sentence may not name anybody: the numbers survive, the
	// names do not exist. A Director-only fact leaking through a sentence would
	// be the loudest oracle on the page precisely because it reads as innocuous
	// copy.
	Why string

	// TallyFree keeps its DENOMINATOR, always. "4 / 5 free" can be read; "4
	// free" cannot.
	TallyFree   string
	TallyPrefer string

	// Outs is the Director-only out-column. EveryoneFree replaces it when
	// nobody is out.
	Outs        []ScheduleStamp
	OutsExtra   int
	Unknowns    []ScheduleStamp
	UnknownsExtra int
	EveryoneFree  bool
}

// ScheduleStamp is one member's swatch + initials in the out column.
//
// KNOWN-BUSY AND NEVER-ANSWERED ARE DIFFERENT MARKS, and that difference is
// ledger #3 enforced in the ink: a member who never saved a week is NOT "out",
// because the tool cannot tell that apart from "busy all week". Busy members get
// a filled swatch under `out`; never-answered members get a HOLLOW swatch under
// `no answer`.
type ScheduleStamp struct {
	Token   string
	Axis    string
	Pattern string
	Hollow  bool
}

// ScheduleCandidateFault is the signed empty/refusal state that replaces the
// three cards. The instrument does not vanish; it says what it does not know.
type ScheduleCandidateFault struct {
	Headline string
	Detail   string
	Chip     string
}

// ScheduleMoreRow is one lower-scoring window in the popover.
type ScheduleMoreRow struct {
	When  string
	Score string
}

// --- S2 · THE MATRIX --------------------------------------------------------

// ScheduleMatrix is members × days, per-person outlines, aggregate density and
// the computed window's bracket. Full width, no side rail beside it — the
// rail's job is done by the Verdict above.
type ScheduleMatrix struct {
	Title string
	Frame string
	Scope string
	Chips []scheduleChip

	// Cols is the column set: days in WEEK zoom, hours in DAY zoom. --cols is
	// emitted SERVER-SIDE as days × slots; the stylesheet never multiplies
	// inside repeat() and no CSS on this surface writes a week length or an hour
	// count.
	Cols []ScheduleCol
	// IdentCap / SayCap are the two sticky ends' headers. IdentCapShort is the
	// PHONE's word for the same column — see the note on Denominator below.
	IdentCap      string
	IdentCapShort string
	SayCap        string

	// Lanes are the per-member rows. EMPTY FOR A PLAYER — including their own —
	// because OverlayMember is omitted wholesale from a player's payload. Their
	// own week is directly above, in the Painter, at the same geometry, and the
	// header says so. No ghost row, no lock, no "+4 hidden".
	Lanes []ScheduleLane
	// Density is the achromatic aggregate; Counts is the exact number.
	Density []ScheduleDensity
	Counts  []ScheduleCount
	// Denominator is the count lane's label and it may NEVER be clipped into
	// "free of 5 in the campai…": it is the sentence that stops the number being
	// read as "of 5 players", which it is not — it includes the Director and
	// everyone who never answered.
	//
	// DenominatorShort is the SAME SENTENCE, SHORTER, for the phone — the
	// drawing's own `free of 5`. The mockup's producer re-runs on resize and can
	// simply swap the string; a page rendered once on the server cannot, so it
	// emits both and one media query chooses. Duplicated words are the price of a
	// width-dependent sentence, and they are cheaper than a truncation that turns
	// this one into the misreading it exists to prevent.
	Denominator      string
	DenominatorShort string
	CountChip        []scheduleChip

	Bracket *ScheduleBracket

	Zero string
	// Caption is ONE flowing paragraph, exactly as the mockup's own
	// `bits.join(' ')` returns one: the marks' key, the two disagreements the
	// numbers cannot state about themselves, and the identity-wrap note when
	// there is one. Split across separate blocks they read as unrelated notes
	// rather than as one key to one grid.
	Caption ScheduleCaption
	Pops    []SchedulePop
}

// ScheduleCol is one matrix column.
type ScheduleCol struct {
	Head string
	Sub  string
	// Major draws the heavier structural rule — the week's own emphasis, not a
	// decoration.
	Major  bool
	DayKey string
	// StartMinute / EndMinute are the column's span in minutes from midnight,
	// which is what the marks inside it are positioned against.
	StartMinute int
	EndMinute   int
	Day         int
}

// ScheduleLane is one member's row in the matrix.
type ScheduleLane struct {
	Token   string
	Name    string
	Axis    string
	Pattern string

	// Cells is populated ONLY when the member has saved something inside the
	// band; the two Note states below replace it otherwise.
	Cells []ScheduleCell
	// Note is the printed sentence that stands where the marks would be.
	// NoteChip rides beside it; NoteWarn switches the register from neutral
	// (an unknown) to warn (a band that is hiding real data).
	//
	// NoteShort is the phone's form. The lane is a single nowrap line, so at 390
	// the long sentence does not merely clip — it clips its own CHIP out of the
	// row with it, and an honesty chip that vanishes on a phone is the one thing
	// this page forbids everywhere it forbids anything.
	Note      string
	NoteShort string
	NoteWarn  string
	NoteChip  []scheduleChip

	// The SAY column: the member's own local clock and their answer.
	LocalTime  string
	NextDay    bool
	Antisocial bool
	ZoneMissing bool
	AskHref    string
	Answer     string
	Tone       string
}

// ScheduleCell is one matrix cell. THE CELL IS THE TARGET (≥24×24 at every
// width); the outline inside it is a MARK, not a control — which is what fixes
// an 8px-tall interactive lozenge.
type ScheduleCell struct {
	Major  bool
	DayKey string
	Axis   string
	Label  string
	PopID  string
	Marks  []ScheduleMark
}

// ScheduleMark is the one new primitive this pass introduces: a hollow,
// pattern-carrying, MINUTE-POSITIONED lozenge.
//
// From / To are percentages across the column, computed server-side from the
// payload's real resolution. The long edges carry the member's locked dash, so
// the mark survives filter:grayscale(1) with no second hue. PREFERRED is the
// same outline filled at 18% of the member's own hue — a LUMINANCE step,
// orthogonal to pattern, because pattern is already spent on WHO and may never
// also mean WHAT.
type ScheduleMark struct {
	From    string
	To      string
	Pattern string
	Prefer  bool
	// ContLeft / ContRight drop the end cap where a window CONTINUES past the
	// column's edge — otherwise every hour boundary in DAY zoom would print a
	// false start and a false finish.
	ContLeft  bool
	ContRight bool
	// Rest is the neutral overflow mark: a 4th window in one cell turns
	// neutral and the exact list is in the popover.
	Rest bool
}

// ScheduleDensity is one column of the achromatic aggregate lane: ONE bar,
// never a stack, never a heat cell.
type ScheduleDensity struct {
	Free   int
	Total  int
	DayKey string
	Major  bool
	Title  string
}

// ScheduleCount is one column of the exact-number lane.
type ScheduleCount struct {
	Free int
	// PeakHour is the hour the count was taken at in WEEK zoom, printed beside
	// the number so the reader knows WHEN.
	PeakHour  string
	Peak      bool
	DayKey    string
	Major     bool
	PopID     string
	Label     string
}

// ScheduleBracket is the computed window drawn over the columns it spans.
//
// ACCENT IS THE TOOL TALKING. A human-offered or human-proposed span would take
// --rule-editorial instead; the accent never colours event data (canon A7).
type ScheduleBracket struct {
	Start int
	End   int
	Label string
	Human bool
}

// SchedulePop is one popover. TOP LAYER, so it escapes the matrix's own
// horizontal scroll container — which is the whole reason a cell's detail is a
// popover and not an inline expansion.
type SchedulePop struct {
	ID    string
	Axis  string
	Head  string
	Rows  []SchedulePopRow
	Foot  string
}

// SchedulePopRow is one line inside a popover.
type SchedulePopRow struct {
	Text string
	Note string
}

// --- S3 · THE ROSTER --------------------------------------------------------

// ScheduleRoster is W-G item 4's permanent home, and the one place a role is
// printed truthfully.
//
// THIS SURFACE PRINTS NO STORED RSVP AGGREGATE ANYWHERE: every number is
// recomputed from the visible, membership-filtered rows.
type ScheduleRoster struct {
	SlotLabel string
	Sub       string
	Chips     []scheduleChip
	// SlotPopID is the Director's slot picker, absent for a player.
	SlotPopID string
	// Rows is EVERY member for a Director and ONLY THE VIEWER'S OWN ROW for a
	// player — no "awaiting reply" column (not derivable for them), no other
	// names, no greyed placeholders.
	Rows    []ScheduleRosterRow
	Caption ScheduleCaption
}

// ScheduleRosterRow is one member row.
type ScheduleRosterRow struct {
	Name    string
	Token   string
	Axis    string
	Pattern string
	Role    string
	IsCoDM  bool
	Host    bool

	// Zone is the IANA leaf with the full identifier in ZoneTitle. When the
	// member has NO zone the signed repair pair renders instead and the clock is
	// LITERALLY EMPTY — never "--:--", never a dash, never a UTC guess.
	Zone      string
	ZoneTitle string
	AskHref   string

	LocalTime  string
	NextDay    bool
	Antisocial bool

	Answer string
	Tone   string
}

// --- S4 · THE PAINTER -------------------------------------------------------

// SchedulePainter is availability ENTRY on the real-world calendar, PERMANENTLY
// DOCKED — not a drawer, not a modal (A3).
//
// The painted weekly grid plus the [This week only | Every week] scope segment
// IS the 2026-07-28 operator directive's "set your normal hours per day without
// retyping everything". "Or they could just type everything in" is honoured by
// the grid being editable per day (exceptions), NOT by a second free-text
// surface.
type SchedulePainter struct {
	Title string
	Frame string
	// ZoneHref is the "Change your zone →" repair. A repair may never be the
	// thing that disappears.
	ZoneHref string

	// Fault replaces the whole form when availability entry cannot be offered.
	// It is a NAMED refusal, never a dashed reserve: dashed means "not built
	// yet" and only that (ledger #21).
	Fault *ScheduleFault
	Form  *SchedulePaintForm
}

// SchedulePaintForm is the painted grid and its controls.
type SchedulePaintForm struct {
	// SaveURL / ExceptionsURL are the SCHEDULER'S OWN SHIPPED WRITE ROUTES.
	// Part B adds neither: forking a second availability write path would fork
	// the composition invariant ("an offer only ever adds") with it.
	SaveURL       string
	ExceptionsURL string
	CSRFToken     string
	Zone          string
	// WeekLabel heads the identity column. It is the WEEK, not the zone: the
	// zone is already stated once in the panel frame, and stating it twice in
	// two registers is how a reader starts checking whether they agree.
	WeekLabel string
	// Axis / Pattern are the VIEWER'S OWN identity, so a mark they paint is the
	// same mark the Director reads in the matrix above — same hue, same locked
	// dash. Colour is never load-bearing alone here either.
	Axis    string
	Pattern string

	Scope        string
	ScopeOptions []ScheduleToggle
	// ScopeNote says what THIS scope's marks mean, in one sentence, beside the
	// control that sets it.
	ScopeNote ScheduleCaption

	Summary string
	Hours   []ScheduleHourHead
	Days    []SchedulePaintDay
	// PrefDays is the same grid for the WOULD PREFER disclosure — the ONE
	// sanctioned open/close on this page, on the viewer's own explicit act.
	PrefDays []SchedulePaintDay
	PrefOpen bool
	PrefNote ScheduleCaption

	Empty string
	// CopyWeek is the Director-only unbuilt affordance. ABSENT for a player,
	// not disabled: scaffolding for a gap nobody is asking that player about is
	// Director-tier, and permission is absence.
	CopyWeek *BenchAction
	Foot     ScheduleCaption
	// Reserve is the ONE dashed band on this whole page and it is reserved for
	// external-calendar import (ledger #21). Director-only, same rule.
	Reserve *ScheduleReserve
}

// ScheduleHourHead is one hour column header in the Painter.
type ScheduleHourHead struct {
	Label string
	Major bool
}

// SchedulePaintDay is one day row of the painted grid.
type SchedulePaintDay struct {
	Label  string
	DayKey string
	// Weekday is 0=Sun..6=Sat, the scheduler's OWN index, emitted server-side.
	//
	// It rides on the row rather than being derived in the driver, and that is
	// not a convenience: the recurring path writes a WEEKDAY and the exception
	// path writes a DATE, and re-deriving either from a printed label ("Mon 20
	// Jul") in JS is precisely how a Monday becomes a Sunday at a locale or
	// timezone boundary.
	Weekday int
	Cells   []SchedulePaintCell
}

// SchedulePaintCell is one hour tick. PER-CELL TOGGLES ARE THE ONLY PATH: no
// drag-select, at any width, on any surface (P8 §10's out-of-scope list). A drag
// gesture cannot be the primary control in a JS-free fragment, and a "fallback"
// primary is a fiction.
type SchedulePaintCell struct {
	ID      string
	Name    string
	Value   string
	Checked bool
	Major   bool
	DayKey  string
	// Hour is the tick's hour-of-day. Emitted for the same reason Weekday is:
	// the driver composes [start,end) MINUTES from it and must not parse them
	// out of the printed value.
	Hour  int
	Label string
}

// ScheduleReserve is the dashed "not built yet" band.
type ScheduleReserve struct {
	Head string
	Body string
}

// --- S5 · THE ANSWER --------------------------------------------------------

// ScheduleAnswer is the in-app twin of the shipped RSVP email actions — a
// RESTYLE, not a build: every action already ships on one endpoint.
//
// A DIRECTOR NEVER SETS SOMEBODY ELSE'S ANSWER. Their half of this surface is a
// read-only responder list over named, membership-filtered rows plus the derived
// AWAITING REPLY group. There is no invitee table, so that group is roster minus
// responders and is derivable for the Director ONLY (ledger #13) — on a player's
// page it does not exist at all.
type ScheduleAnswer struct {
	Director bool

	Title string
	Sub   string
	Chips []scheduleChip
	// Unanswered marks the player's own "you haven't answered" badge.
	Unanswered bool

	Headline string
	// HeadlineTone is "ok" | "warn" | "".
	HeadlineTone string

	// Form is the player's tri-state. It posts to the EXISTING Player+ RSVP
	// route; Part B adds no answer route.
	Form *ScheduleAnswerForm

	// Rows / Awaiting are the Director's two groups.
	Rows     []ScheduleRosterRow
	Awaiting []ScheduleRosterRow
	Notes    map[string]string

	Caption ScheduleCaption
	Foot    string
}

// ScheduleAnswerForm is the player's tri-state and its two docks.
type ScheduleAnswerForm struct {
	Action    string
	CSRFToken string
	// My is the viewer's own stored answer, printed as a headline rather than as
	// a fourth button state — the signed trio is three buttons and stays three.
	My      string
	Options []ScheduleAnswerOption
	// OutWeekPopID is the confirm popover for "Out just this week" — the same
	// GET-confirm hygiene the token flow has, because that control writes TWO
	// things.
	OutWeekPopID string
	SuggestHref  string
	SuggestOpen  bool
	SuggestNote  ScheduleCaption
}

// ScheduleAnswerOption is one word of the tri-state. WORDS, NEVER GLYPHS.
type ScheduleAnswerOption struct {
	Value   string
	Label   string
	Pressed bool
}
