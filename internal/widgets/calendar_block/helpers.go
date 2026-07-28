// helpers.go — pure, side-effect-free derivations for the Block's templ
// components. Everything here is a function of BlockData alone: the Block
// performs no queries, holds no request state, and never reaches for a clock
// (widgetbindings renders bound Blocks with context.Background(),
// app/routes.go:2929/2941).
//
// Two jobs live here and nothing else:
//
//  1. VALIDATION AT THE BOUNDARY. Every value that reaches a `style` attribute
//     is whitelisted before it gets there. templ's own style sanitizer would
//     catch an outright injection, but a silently-dropped declaration is a
//     rendering bug that looks like a data bug, so the widget normalises to a
//     known-good token instead of trusting its producer.
//
//  2. THE HONESTY GUARANTEES, expressed as code rather than as convention —
//     most importantly that the sync pill's DENOMINATOR CAN NEVER DROP, whatever
//     the producer hands over.
package calendar_block

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ── channel whitelists ──────────────────────────────────────────────────────
//
// Three colour channels exist and they never mix (canon A7):
//
//	--axis    the event axis. The marks layer is the ONLY layer that may read it
//	          and it is FORBIDDEN from referencing --accent.
//	--cal     calendar identity (the Nameplate dot, the mini rail).
//	--bandhue the era tint, driven by the ERA and never hardcoded per calendar
//	          (calendar-v4.html:1566 hardcodes Harptos and is a known defect).
//
// None of the three is @property-registered as animatable. That is a recorded
// REFUSAL (canon A7), not an omission.
var (
	// A CSS custom-property reference, an sRGB hex, or an oklch() literal built
	// only from characters that cannot terminate a declaration.
	reVarRef = regexp.MustCompile(`^var\(--[a-zA-Z0-9_-]+\)$`)
	reHex    = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)
	reOklch  = regexp.MustCompile(`^oklch\([0-9a-zA-Z.%/ +-]+\)$`)
	rePat    = regexp.MustCompile(`^p[1-8]$`)
	reLength = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?px$`)
)

// AxisFallback is what an unrecognised axis value resolves to: the neutral
// "no owner" grey. A mark with a broken axis still renders, still carries its
// pattern, and still survives greyscale — it just stops claiming a hue.
const AxisFallback = "var(--own-none)"

// colourToken normalises any of the three colour channels.
func colourToken(v, fallback string) string {
	v = strings.TrimSpace(v)
	if reVarRef.MatchString(v) || reHex.MatchString(v) || reOklch.MatchString(v) {
		return v
	}
	return fallback
}

func axisToken(v string) string { return colourToken(v, AxisFallback) }
// calHueTokens is the CLOSED set of --cal channel tokens. CalHue is a TOKEN
// NAME ("harptos"), never a colour value — the producer picks from this set
// (blockCalHue) and the stylesheet defines a --cal-<token> for each.
//
// The whitelist discipline is unchanged and still the point: a bare value must
// never reach a style attribute. What changed is WHAT is whitelisted. Accepting
// only colour values, as this did, greyed out every real calendar's identity,
// because a token name is not a colour and fell through to the fallback.
//
// This set is MIRRORED from the producer and cannot see it. TestCalToken_
// MatchesStylesheet catches half the drift — a token here with no --cal-<token>
// in the CSS. The other half (the producer adding a token this set lacks) is
// caught by the cross-layer seam test, which is why that test exists.
var calHueTokens = map[string]bool{
	"harptos": true,
	"real":    true,
	"elven":   true,
	"dwarven": true,
}

// calToken maps a --cal token name to its channel, falling back to the neutral
// structural rule for anything unrecognised.
func calToken(v string) string {
	if calHueTokens[strings.TrimSpace(v)] {
		return "var(--cal-" + strings.TrimSpace(v) + ")"
	}
	return "var(--rule-structural-strong)"
}
func bandToken(v string) string { return colourToken(v, "var(--rule-structural)") }

// patternClass normalises a stroke pattern to p1..p8.
//
// The pattern — not the hue — is the GREYSCALE IDENTITY CHANNEL. p1 solid ·
// p2 dash 4-2 · p3 dot 1-2 · p4 dash 2-2 · p5 double hairline · p6 dash 6-2 ·
// p7 solid+notch · p8 fine dash 1-1. p7 and p8 are unassigned headroom.
// Defaulting to p1 is safe: it is the solid stroke, which is legible under
// every filter.
func patternClass(p string) string {
	if rePat.MatchString(strings.TrimSpace(p)) {
		return strings.TrimSpace(p)
	}
	return "p1"
}

// cssLength normalises a length to a plain px value, or returns fallback.
func cssLength(v, fallback string) string {
	v = strings.TrimSpace(v)
	if reLength.MatchString(v) {
		return v
	}
	return fallback
}

// ── style-attribute builders ────────────────────────────────────────────────

// blockStyle carries the Block's three inherited scalars.
//
// --week-len is written PER MONTH and defaults to 10 in the stylesheet. Ten-day
// weeks are native; there is no literal 7 anywhere in the grid CSS.
// --month-len is the day count, used by the sub-mini tick rule.
func blockStyle(d BlockData) string {
	week := d.Month.WeekLen
	if week < 1 {
		week = 10
	}
	days := d.Month.Days
	if days < 1 {
		days = 1
	}
	return fmt.Sprintf("--cal:%s;--week-len:%d;--month-len:%d", calToken(d.CalHue), week, days)
}

// NOTE ON THE WEEK-NUMBER GUTTER. It is column 1 of the SAME grid and every row
// is a subgrid across it, so header / era band / cells / intercalary can never
// drift out of alignment. Its WIDTH is not decided here: the mockup drops the
// gutter below 481px of host, and that is a host measurement the producer cannot
// make. The Block emits `data-weeknums` (the viewer's layer choice, a fact) and
// the stylesheet opens the track only where it fits — see §GUTTER in
// calendar-block.css. Writing --gut inline would beat the container query and
// push a 20px track onto a 358px phone.
//
// The mockup ALSO gates the gutter on `m.week === 10`. Wave 1 does not: a
// literal week length in the renderer is exactly what this brief exists to
// remove, and "does a week number mean anything on this calendar" is a producer
// decision expressed by enabling the layer.

// bandStyle places one era band inside its week row's subgrid.
//
// StartCol is 1-based within the week; +1 steps over the gutter track.
func bandStyle(b EraBand) string {
	start := b.StartCol
	if start < 1 {
		start = 1
	}
	span := b.Span
	if span < 1 {
		span = 1
	}
	return fmt.Sprintf("grid-column:%d/span %d;--bandhue:%s", start+1, span, bandToken(b.BandHue))
}

// halfRuleStyle places the five-column ruler at the half boundary.
func halfRuleStyle(col int) string {
	return fmt.Sprintf("grid-column:%d/span 1", col+1)
}

// axisStyle is the only style a mark ever carries.
func axisStyle(a string) string { return "--axis:" + axisToken(a) }

// ── the five-column rule ────────────────────────────────────────────────────

// halfColumn returns the 1-based column the strong ramp step falls after, or 0
// when this week length has none.
//
// The rule is the producer's to decide (Weekday.Half / DayCell.Half /
// EraBand.Half): humans cannot count to ten across identical columns and 5+5 is
// instant, so it applies to ten-day weeks and not to seven-day ones. This
// function READS that decision rather than re-deriving it, which is how the
// grid stays free of a literal week length.
func halfColumn(m MonthGeometry) int {
	for _, w := range m.Weekdays {
		if w.Half {
			return w.Index + 1
		}
	}
	for _, r := range m.Rows {
		for _, c := range r.Cells {
			if c.Half {
				return c.Col
			}
		}
	}
	return 0
}

// ── layers ──────────────────────────────────────────────────────────────────

// hasLayer reports whether a layer key is switched on for this viewer.
//
// Valid keys: moons · eras · weeknums · ledger · moongraph · legend · horizon ·
// shelf. The first three are INSIDE layers — they change the month's geometry
// and therefore apply instantly and silently (canon A8 / L-M2). DEF is
// ["moons"]: the default surface is a month with its moon phases and nothing
// else.
func hasLayer(l LayerState, key string) bool {
	for _, k := range l.Enabled {
		if k == key {
			return true
		}
	}
	return false
}

// ── the sync pill ───────────────────────────────────────────────────────────

// syncStateClass maps the five honesty states onto their pill class. An
// unrecognised state resolves to the muted "paused" treatment rather than to a
// green one: a badge that says nothing is honest, a badge that says "in sync"
// without knowing is the exact lie this pill exists to prevent.
func syncStateClass(state string) string {
	switch state {
	case "ok":
		return "s-ok"
	case "drift":
		return "s-drift"
	case "bad":
		return "s-bad"
	default:
		return "s-pause"
	}
}

// syncHasDot reports whether the pill leads with its state dot. The two states
// that describe an ABSENCE of live sync — paused and not-linked — carry no dot,
// matching the signed strings (calendar-v4.html:1641-1643).
func syncHasDot(state string) bool {
	return state == "ok" || state == "drift" || state == "bad"
}

// syncDenominator is the phrase the pill may never render without.
func syncDenominator(s SyncPill) string {
	return fmt.Sprintf("%d of %d linked", s.Linked, s.Total)
}

// SyncFullText is the wide string. SyncCompactText is the narrow one. BOTH are
// emitted on every render and a container query chooses — full tier shows Full,
// std shows Compact, mini and sub-mini show neither. A Go-side tier decision
// would be wrong by construction: the producer does not know the host width.
//
// THE DENOMINATOR NEVER DROPS; only transport and timestamp do. If a producer
// hands over an empty string, or a string that has lost its denominator, these
// repair it rather than rendering the lie. That is not defensive padding — a
// green "In sync" with no denominator is the precise failure the operator hit.
func SyncFullText(s SyncPill) string {
	return withDenominator(s.Full, s)
}

// SyncCompactText is the compact form. It drops transport and timestamp and
// keeps the count.
func SyncCompactText(s SyncPill) string {
	return withDenominator(s.Compact, s)
}

func withDenominator(text string, s SyncPill) string {
	text = strings.TrimSpace(text)
	// "1 of 4" is the compact denominator; "1 of 4 linked" the full one. Either
	// satisfies the guarantee.
	if text != "" && strings.Contains(text, fmt.Sprintf("%d of %d", s.Linked, s.Total)) {
		return text
	}
	if text == "" {
		text = syncFallbackLabel(s.State)
	}
	return text + " · " + syncDenominator(s)
}

func syncFallbackLabel(state string) string {
	switch state {
	case "ok":
		return "In sync"
	case "drift":
		return "Drifted"
	case "bad":
		return "Incompatible structure"
	case "pause":
		return "Paused"
	default:
		return "Not linked"
	}
}

// ── marks ───────────────────────────────────────────────────────────────────

const (
	namedChipCap = 3 // a named cell is always exactly 84px; chip count may not change a row's height
	underlineCap = 3 // ONE bar, never a stack — a stack encodes a magnitude the data does not have
)

// chipsFor returns the marks a named cell prints as chips.
//
// Three chips, or two chips plus a "+n more" row — never both, so a named cell
// is always exactly 84px and chip count can never change a row's height. The
// producer normally trims and sets MoreCount; this re-derives the split so an
// untrimmed producer still renders a fixed-height cell.
func chipsFor(c DayCell) []Mark {
	cap := namedChipCap
	if moreCount(c) > 0 {
		cap = namedChipCap - 1
	}
	if len(c.Marks) <= cap {
		return c.Marks
	}
	return c.Marks[:cap]
}

// moreCount is the overflow the cell declares, including any marks the producer
// left in place beyond the chip cap.
func moreCount(c DayCell) int {
	n := c.MoreCount
	if over := len(c.Marks) - namedChipCap; over > 0 && n == 0 {
		// An untrimmed producer: 4 marks means 2 chips + "+2 more".
		n = len(c.Marks) - (namedChipCap - 1)
	}
	if n < 0 {
		return 0
	}
	return n
}

// underlineSegs returns the segments of a presence underline: at most three,
// and the last turns neutral when there are more events than segments. The
// exact count lives in the Ledger, never in the bar.
//
// NOTE FOR PRODUCERS: because density is a container query, the producer cannot
// know which subtree will be visible and therefore MUST NOT trim Marks to a
// density's cap. Hand over what the viewer may see; the two subtrees each apply
// their own ceiling. MoreCount is for events the producer itself dropped.
func underlineSegs(c DayCell) []Mark {
	if len(c.Marks) <= underlineCap {
		return c.Marks
	}
	return c.Marks[:underlineCap]
}

// underlineRestAt reports whether segment i is the neutral overflow segment.
// The day's total is len(Marks) — MoreCount is OVERLAPPING (a chip fold within
// that list, data.go), so adding it here claimed events that do not exist.
func underlineRestAt(c DayCell, i int) bool {
	shown := len(underlineSegs(c))
	return i == shown-1 && len(c.Marks) > shown
}

// totalMarks counts every mark the viewer can see in this month, including the
// intercalary row. Used only by the mini / sub-mini foot line, which states a
// count it can actually derive rather than a "next event" it cannot.
//
// A day's total is len(Marks), NEVER len(Marks)+MoreCount: MoreCount is
// OVERLAPPING (data.go) — it counts marks already in the list that are not
// drawn as chips, so adding it double-counts the folded tail and the foot
// printed "10 events" for a 7-event month.
func totalMarks(d BlockData) int {
	n := 0
	for _, r := range d.Month.Rows {
		for _, c := range r.Cells {
			n += len(c.Marks)
		}
	}
	for _, ic := range d.Month.Intercalary {
		n += len(ic.Marks)
	}
	return n
}

// markTitle is the hover string for a mark: its own title, plus the audience
// label when the viewer is entitled to see one.
//
// PERMISSION IS ABSENCE. For a player the AudienceMark is nil and the mark is an
// ordinary mark, with no hint that anyone else cannot see it — no placeholder,
// no ghost, no greyed row.
func markTitle(m Mark) string {
	if m.Audience == nil || m.Audience.Label == "" {
		return m.Title
	}
	return m.Title + " — " + m.Audience.Label
}

// ── moons ───────────────────────────────────────────────────────────────────

// moonCap is the grid's ceiling. However many moons a calendar declares, the
// grid draws at most this many so the month can never grow with the fiction.
const moonCap = 3

// moonsFor caps the discs a cell draws.
func moonsFor(c DayCell) []MoonDisc {
	if len(c.Moons) <= moonCap {
		return c.Moons
	}
	return c.Moons[:moonCap]
}

// moonStyle derives the terminator geometry for one disc.
//
// A real terminator, not a pie fill (L25): the lit half plus an ellipse of width
// |cos(2πφ)| that SUBTRACTS for a crescent and ADDS for a gibbous. Illum alone
// determines it — illum = (1 − cos 2πφ)/2, so |cos 2πφ| = |1 − 2·illum| — which
// is why this needs no phase angle and cannot disagree with the disc's fill.
//
// MoonDisc.Terminator is honoured when the producer supplies a plain px length;
// otherwise it is derived. Either way the widget never emits an unvalidated
// length into a style attribute.
func moonStyle(md MoonDisc) string {
	const discPx = 10.0
	illum := md.Illum
	if math.IsNaN(illum) {
		illum = 0
	}
	illum = math.Max(0, math.Min(1, illum))
	derived := strconv.FormatFloat(math.Abs(1-2*illum)*discPx, 'f', 1, 64) + "px"
	term := cssLength(md.Terminator, derived)
	fill := "var(--surface-card)"
	if illum > 0.5 {
		fill = "var(--rule-structural-strong)" // gibbous: the ellipse ADDS
	}
	return "--term:" + term + ";--termfill:" + fill
}

// moonWaneClass flips the terminator to the other limb for a waning moon.
func moonWaneClass(md MoonDisc) string {
	if md.Waxing {
		return "ph"
	}
	return "ph wane"
}

// moonTitle names the body, its illumination and — when the producer marks it —
// that it is in shadow. Eclipse is stated in words rather than drawn: the node
// window is a bracket in the Sky Register treatment, which is not wave 1's
// default surface, and inventing a disc treatment for it would be design.
func moonTitle(md MoonDisc) string {
	pct := int(math.Round(math.Max(0, math.Min(1, md.Illum)) * 100))
	s := fmt.Sprintf("%s %d%%", md.Name, pct)
	if md.Eclipse {
		s += " · in shadow"
	}
	return s
}

// ── small text helpers ──────────────────────────────────────────────────────

// tieClass stamps the per-mark tie state so CSS can change INK without the
// server changing membership (pin r51).
//
// TieMode never removes a mark from a cell: the producer emits the whole
// viewer-visible set in both modes and flags each one. Dropping untied marks
// would change a cell's contents and therefore its height — the no-motion
// violation the toggle cannot survive — and would leave a CSS-only toggle
// nothing to re-ink, because CSS cannot restore a mark that was never sent.
//
// Off an entity page there is nothing to be tied to, so every mark is emitted
// unclassed and the toggle does not render at all.
func tieClass(m Mark) string {
	if m.Tied {
		return " tied"
	}
	return " untied"
}

// calLetter is the calendar's single-character identity mark — the third
// channel beside hue and pattern.
//
// Colour is never the only identity channel, and neither is colour+pattern: a
// pattern is legible on a stroke but a 8px dot cannot carry one at every tier.
// The letter survives greyscale, low resolution and a washed-out projector.
// Truncated to one rune because the surface budgets exactly one.
func calLetter(s string) string {
	for _, r := range strings.TrimSpace(s) {
		return string(r)
	}
	return ""
}

func intText(n int) string { return strconv.Itoa(n) }

// ── the CSS-only tie toggle's identifiers ───────────────────────────────────
//
// The toggle is a hidden radio pair (nameplate.templ) whose :checked state a
// `:has()` rule reads to change the untied ink level. That needs two things Go
// has to supply: a radio GROUP NAME unique to this Block, and a stable ID per
// option so each <label> can address its own input.
//
// Unique per (calendar, host entity) — NOT a package counter and NOT a random
// value. A page may compose more than one Block, and two Blocks sharing a radio
// name would fight over one piece of state; but the same Block re-rendered by an
// HTMX binding swap must keep the SAME name, or the swapped-in fragment loses
// the mode the viewer had just chosen. A pure function of the data satisfies
// both, and it stays identical across servers, which a counter would not.

// tieGroupName is the radio group shared by this Block's two tie options.
func tieGroupName(d BlockData) string {
	return "tie-" + domToken(d.CalendarSlug) + "-" + domToken(d.Viewer.HostEntity)
}

// tieInputID is one tie option's DOM id ("…-tied" / "…-whole").
func tieInputID(d BlockData, mode string) string {
	return tieGroupName(d) + "-" + domToken(mode)
}

// domToken reduces an arbitrary identifier to characters that are safe in an
// id/name and in the attribute selectors the stylesheet writes. Chronicle's ids
// are UUIDs and already safe; this exists so a slug that is not cannot break the
// markup, and so an empty identity still yields a usable token.
func domToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "x"
	}
	return b.String()
}

// eventCountLabel keeps the foot line grammatical without a template branch.
func eventCountLabel(n int) string {
	if n == 1 {
		return "1 event"
	}
	return strconv.Itoa(n) + " events"
}

// layersInvokerTitle says out loud that the switchboard is not built yet, so an
// inert control reads as a reserved affordance rather than as a bug.
func layersInvokerTitle(l LayerState) string {
	if l.HasSwitchboard {
		return "Layers"
	}
	return "Layers — needs backend"
}

// wdLong / wdShort are the weekday header's two lengths. Both are emitted and
// the container query picks: three characters at full tier, two at std, none at
// mini and below. Rune-safe, because a calendar's weekday names are the most
// likely place in this widget to meet a non-ASCII abbreviation.
func wdLong(w Weekday) string { return truncRunes(firstNonEmpty(w.Abbr, w.Name), 3) }

func wdShort(w Weekday) string { return truncRunes(firstNonEmpty(w.Abbr, w.Name), 2) }

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ── keys ────────────────────────────────────────────────────────────────────

// dayKey is the ANSWER key every dated cell and row carries (COMMON §6.6,
// guard B4). Nothing consumes it in wave 1; W-B does. A surface that forgets it
// simply stops answering later, and that is invisible in code review.
func dayKey(slug string, day int) string {
	if slug == "" {
		slug = "cal"
	}
	return slug + "-" + strconv.Itoa(day)
}

// intercalaryKey distinguishes an intercalary day from an ordinary one of the
// same ordinal — Midwinter 1 is not Deepwinter 1.
func intercalaryKey(slug string, day int) string {
	if slug == "" {
		slug = "cal"
	}
	return slug + "-i" + strconv.Itoa(day)
}

// ── the sub-mini tick rule ──────────────────────────────────────────────────

// tickDay is one column of the thirty-tick month rule the sub-mini tier draws
// instead of a grid. Dropping the grid at 220px is honest; squeezing ten columns
// into it is not.
type tickDay struct {
	Day     int
	Axis    string
	HasMark bool
	IsToday bool
	Fogged  bool
}

// tickDays flattens the month into its ordinal days.
func tickDays(d BlockData) []tickDay {
	out := make([]tickDay, 0, d.Month.Days)
	seen := make(map[int]DayCell, d.Month.Days)
	for _, r := range d.Month.Rows {
		for _, c := range r.Cells {
			if c.Day > 0 {
				seen[c.Day] = c
			}
		}
	}
	for day := 1; day <= d.Month.Days; day++ {
		c, ok := seen[day]
		t := tickDay{Day: day}
		if ok {
			t.IsToday = c.IsToday
			t.Fogged = c.Fogged
			if len(c.Marks) > 0 {
				t.HasMark = true
				t.Axis = axisToken(c.Marks[0].Axis)
			}
		}
		out = append(out, t)
	}
	return out
}

func tickClass(t tickDay) string {
	cls := make([]string, 0, 3)
	if t.HasMark {
		cls = append(cls, "ev")
	}
	if t.IsToday {
		cls = append(cls, "now")
	}
	if t.Fogged {
		cls = append(cls, "fg")
	}
	return strings.Join(cls, " ")
}

// ── cell / header classes ───────────────────────────────────────────────────

func cellClass(c DayCell, week int) string {
	cls := []string{"cell"}
	if c.Day == 0 {
		cls = append(cls, "out")
	}
	if c.Half {
		cls = append(cls, "half")
	}
	if week > 0 && c.Col == week {
		cls = append(cls, "lastcol")
	}
	// Fogged is wave-1-dead by ruling: there is no queryable knowledge horizon
	// on main, so producers leave it false and the .cell.fog rule ships unused.
	// The branch stays so W-F does not have to re-touch every cell.
	if c.Fogged {
		cls = append(cls, "fog")
	}
	if c.Intercalary {
		cls = append(cls, "ic")
	}
	return strings.Join(cls, " ")
}

func weekdayClass(w Weekday) string {
	if w.Half {
		return "half"
	}
	return ""
}

func bandClass(b EraBand) string {
	cls := []string{"band"}
	if b.Edge {
		cls = append(cls, "edge")
	}
	// The mockup DECLARES .band.half and never applies it to a band. Wave 1
	// applies it (see instrument.templ's .halfrule note) — a ramp step that
	// stops at the era band is a visible seam.
	if b.Half {
		cls = append(cls, "half")
	}
	if b.OpenLeft {
		cls = append(cls, "contL")
	}
	if b.OpenRight {
		cls = append(cls, "contR")
	}
	return strings.Join(cls, " ")
}

// weekLabel is the gutter's week number. It is a POSITION in the month, not a
// date, so it carries no data-day.
func weekLabel(r WeekRow) string {
	return "W" + strconv.Itoa(r.Index+1)
}

// ── the three GM marks ──────────────────────────────────────────────────────
//
// Three unrelated conditions, three distinct marks, one printed legend:
//
//	dogear   a filled gold notch, top-right  — a dm_only event is on this day
//	audmark  a gold DIAMOND, bottom-right    — a restricted audience is on this day
//	                                           (never a circle: circles are moons, L22)
//	fog      a flat surface step             — past the knowledge horizon
//
// Both gold marks are GM/co-DM only, and they are absent rather than greyed for
// everyone else. For a player the producer leaves AudienceMark nil and the mark
// is an ordinary mark, with no hint that anyone else cannot see it — permission
// is ABSENCE, not a placeholder.
//
// WAVE-1 RULING: composed tag+member audiences do not exist on main. The only
// two things that do are `visibility == dm_only` (Restricted false) and a
// `visibility_rules` restriction (Restricted true), which is exactly the split
// these two predicates read.

// cellHasGMOnly reports whether a day carries an event hidden from players.
func cellHasGMOnly(c DayCell) bool {
	for _, m := range c.Marks {
		if m.Audience != nil && !m.Audience.Restricted {
			return true
		}
	}
	return false
}

// cellHasRestricted reports whether a day carries an event with a restricted
// audience.
func cellHasRestricted(c DayCell) bool {
	for _, m := range c.Marks {
		if m.Audience != nil && m.Audience.Restricted {
			return true
		}
	}
	return false
}

// gmMarkTitle names what the notch or diamond stands for, in the viewer's own
// terms. Never a count the widget cannot derive.
func gmMarkTitle(c DayCell, restricted bool) string {
	for _, m := range c.Marks {
		if m.Audience != nil && m.Audience.Restricted == restricted && m.Audience.Label != "" {
			return m.Audience.Label
		}
	}
	if restricted {
		return "Restricted"
	}
	return "GM only"
}

// ── the tie toggle ──────────────────────────────────────────────────────────

// tieMode normalises the viewer's tie mode. It is emitted as a data attribute
// on the Block rather than baked into per-cell classes, so W-C's CSS-only
// toggle has one attribute to flip and no Go decision to undo.
//
// TieMode changes ink LEVEL only. No cell may grow, move, or leave the DOM when
// it flips — that is what makes it legal under the no-motion rule and what makes
// TiedCount and WholeCount non-differenceable.
func tieMode(v ViewerContext) string {
	if v.HostEntity == "" {
		return ""
	}
	if v.TieMode == "tied" {
		return "tied"
	}
	return "whole"
}

// eraBadgeText is the Nameplate's era stamp: the era plus the year it resolves
// in. Empty when the calendar declares no eras — in which case the producer has
// normally set Fault instead, and the date line says so.
func eraBadgeText(d BlockData) string {
	if d.EraLabel == "" {
		return ""
	}
	return d.EraLabel + " · " + strconv.Itoa(d.Month.Year)
}

// moonsBadgeText is the Nameplate's declared-moon statement — the r51
// acceptance line made visible: a calendar declaring MORE moons than the grid
// draws states the total ("3 of 4 moons"), and one declaring three or fewer
// states nothing extra. The ceiling is moonCap, declared ONCE here and never
// per cell (L25/L30), and the number is MonthGeometry.MoonsDeclared — the
// per-cell Moons are already capped (data.go) and cannot supply it.
//
// The badge is layer-gated with the discs it explains (the signed MOONS_ON()
// gate): with the moons layer off the grid draws no moons at all, and
// "3 of 4" would be a claim about a surface that is not there.
func moonsBadgeText(d BlockData) string {
	if !hasLayer(d.Layers, "moons") || d.Month.MoonsDeclared <= moonCap {
		return ""
	}
	return fmt.Sprintf("%d of %d moons", moonCap, d.Month.MoonsDeclared)
}

// moonsBadgeTitle explains the ceiling on hover. The signed title's "all of
// them are in the Almanac" tail is NOT ported: the Almanac is W-E and does not
// exist yet, and a hover that points at an unbuilt surface is a small lie.
func moonsBadgeTitle(d BlockData) string {
	return fmt.Sprintf("%d moons declared; the grid draws %d", d.Month.MoonsDeclared, moonCap)
}

// ── the ANSWER ladder: day selection, in CSS, with no JS and no route ───────
//
// THE DOCKED LEDGER'S WHOLE PROMISE is that choosing a day repaints a panel
// that was already on screen: nothing appears from nowhere and nothing reflows
// (canon A3 struck D9's slide-in drawer clause on exactly this ground). A
// network round trip would put latency inside that promise, so selection rides
// the initial render and resolves in the stylesheet (CTS-1, SIGNED: Option A).
//
// It could not be JS in any case — boot.js:163 sets
// `htmx.config.allowScriptTags = false`, so a <script> inside an HTMX-swapped
// fragment never executes — and ANSWER-on-hover forces a static per-day rule
// set regardless of how SELECTION is implemented, because a hover cannot be
// served by a fetch. Once that ladder exists, selection is one more rule per
// day and a route buys nothing.
//
// WHAT THE LADDER IS. CSS cannot compare two dynamic attribute values, so there
// is one static rule set per DAY ORDINAL, generated into the stylesheet between
// the BEGIN/END markers by answerLadderCSS() below and diff-tested by
// TestCSS_AnswerLadderIsGenerated. Hand-writing it is how it would rot.
//
// TWO KEY NAMESPACES, and forgetting the second is guard B4's failure mode one
// level up: an intercalary day's key is `slug-iN` (intercalaryKey), not
// `slug-N`, so a ladder keyed on the ordinal alone would make Midwinter
// silently stop answering. The ladder enumerates both.

const (
	// answerLadderDays is the ladder's ceiling for ordinary days and
	// answerLadderIntercalary for intercalary ones (CTS-2, SIGNED).
	//
	// PAST THE BOUND A DAY CARRIES NO SELECTION CONTROL AT ALL — no radio, no
	// label, no dead affordance. That is the house honesty idiom (WG-spec V18:
	// a misconfigured thing prints its fault where its value would go, and a
	// `title` is not an honesty mechanism), and it beats a control that
	// silently does nothing. The Ledger still LISTS every day; only the
	// answering stops, and only past the bound.
	//
	// Mirrored in the stylesheet by the generated block. TestCSS_AnswerLadder
	// AgreesWithGo asserts the two cannot drift.
	answerLadderDays        = 40
	answerLadderIntercalary = 8
)

// answerLadderMarkers bracket the generated block in the stylesheet. The repo's
// existing snapshot idiom (UPDATE_ROUTES_SNAPSHOT, UPDATE_SANITIZE_SNAPSHOT).
const (
	answerLadderBegin = "/* BEGIN ANSWER LADDER — generated */"
	answerLadderEnd   = "/* END ANSWER LADDER */"
)

// dayOrdKey is a day's LADDER key: the bare ordinal, calendar-independent,
// because the ladder lives in ONE static sheet shared by every calendar.
// dayKey's `slug-N` cannot be used for it — the sheet cannot know the slug.
func dayOrdKey(day int) string { return strconv.Itoa(day) }

// intercalaryOrdKey is the second namespace. Midwinter 1 is not Deepwinter 1,
// and a suffix ladder keyed on "-1" would match both.
func intercalaryOrdKey(day int) string { return "i" + strconv.Itoa(day) }

// dayAnswers / intercalaryAnswers report whether an ordinal is inside the
// ladder's bound and therefore gets a selection control at all.
func dayAnswers(day int) bool { return day >= 1 && day <= answerLadderDays }

func intercalaryAnswers(day int) bool { return day >= 1 && day <= answerLadderIntercalary }

// answerLadderKeys enumerates BOTH namespaces in the order the sheet writes
// them. The one place the two bounds are turned into keys, so the markup, the
// sheet and the tests cannot disagree about what the ladder covers.
func answerLadderKeys() []string {
	keys := make([]string, 0, answerLadderDays+answerLadderIntercalary)
	for d := 1; d <= answerLadderDays; d++ {
		keys = append(keys, dayOrdKey(d))
	}
	for d := 1; d <= answerLadderIntercalary; d++ {
		keys = append(keys, intercalaryOrdKey(d))
	}
	return keys
}

// dayPickAll is the radio group's explicit "none" option. A radio cannot be
// un-checked by another radio, so the Ledger head's reserved `✕ all` slot is
// this option's LABEL — which is why that slot is reserved rather than
// conditionally rendered (design notes §10 defect 5: a conditional chip made
// .lhead wrap and jolted the Ledger on the commonest interaction).
const dayPickAll = "all"

// dayPickGroupName is the radio group shared by this Block's day options.
//
// A PURE FUNCTION OF THE DATA, exactly as tieGroupName is and for the same two
// reasons: a page may compose more than one Block (the Bench composes four) and
// two Blocks sharing a radio name would fight over one piece of state; but the
// SAME Block re-rendered by an HTMX binding swap must keep the same name, or
// the swapped-in fragment loses the day the viewer had just chosen. A counter
// satisfies the first and breaks the second, and would differ between servers.
func dayPickGroupName(d BlockData) string {
	return "day-" + domToken(d.CalendarSlug) + "-" + domToken(d.Viewer.HostEntity)
}

// dayPickInputID is one day option's DOM id, addressed by its stretched label.
func dayPickInputID(d BlockData, key string) string {
	return dayPickGroupName(d) + "-" + domToken(key)
}

// dayPickLabel is the option's accessible name. The radios are visually hidden,
// so this is the only name a screen reader or a keyboard user ever gets.
func dayPickLabel(name string, day int) string {
	if name != "" {
		return name
	}
	return "Day " + strconv.Itoa(day)
}

// ledgerDocked reports whether the Ledger zone renders for this viewer: the
// layer registry says yes AND the host has not removed it.
//
// THE DAY-SELECTION CONTROLS ARE GATED ON THE SAME PREDICATE, in one place, so
// the two cannot drift. A Block with no Ledger has nothing for a chosen day to
// repaint and no home for the `✕ all` option that deselects it, so shipping the
// controls anyway would be shipping a radio group with no way out of it.
func ledgerDocked(d BlockData) bool {
	return hasLayer(d.Layers, "ledger") && !d.Ledger.Hidden
}

// answerLadderCSS renders the generated ladder block, markers included.
//
// THREE RULES PER KEY, and the third is the ANSWER primitive:
//
//  1. filter — every Ledger row that is not this day leaves the flow. This is
//     the ONE sanctioned content change in the Block and it is bounded to
//     .lrows; the month grid never moves (2026-07-27 motion policy).
//  2. reveal — this day's head-context line and its "nothing on this day"
//     line, which are rendered for every day and hidden until chosen. CSS
//     cannot compute "3 Deepwinter · 1 event", so the server renders all of
//     them and the ladder picks one.
//
//     IT NAMES ITS TWO SURFACES AND REVERTS RATHER THAN SETTING A BOX, and
//     both halves of that are a fix, not a style. The rule was first written
//     as `.lday[data-lday="N"] { display: block }` — matching on a class token
//     that .lrow ALSO carried — so choosing a day turned every listed row from
//     the signed one-line flex row into a stack with the time drawn over the
//     day numeral. It survived review because .lrow's height is FIXED, so the
//     collapse changed no geometry any probe was reading (the fix round on
//     C-CALV4-LEDGER-P6; the row lost the token in the same commit, and
//     TestProbe_LedgerHeightIsInvariantUnderSelection now reads computed
//     `display` off a real engine).
//
//     `revert` rather than `block` because these two surfaces have DIFFERENT
//     natural boxes — .lctx is a span in the head, .lzero a div in the rows —
//     and a reveal rule that has to know which is a reveal rule that can be
//     wrong. Reverting to the UA default un-hides each one as itself and
//     asserts nothing about layout, which is precisely the claim the ladder is
//     allowed to make.
//  3. answer — a hovered or focused day cell lights the matching Ledger row and
//     vice versa, by setting --answer on the PARTNER. The M1 region rule falls
//     out of the selector's own shape: the partner is always drawn from the
//     OTHER region (.grid ↔ .lrows), never from the origin's own, so hovering
//     one Ledger row cannot light the five siblings beside it.
//
// The answered LOOK is not repeated 48 times. Rule 3 sets one inherited custom
// property and the hand-written §ANSWER rules read it, so every value the
// ANSWERED state changes lives in one auditable place — which is also what
// keeps the motion budget's allowlist small enough to be a real allowlist.
func answerLadderCSS() string {
	var b strings.Builder
	b.WriteString(answerLadderBegin)
	b.WriteString("\n/* ")
	b.WriteString(strconv.Itoa(answerLadderDays))
	b.WriteString(" ordinary + ")
	b.WriteString(strconv.Itoa(answerLadderIntercalary))
	b.WriteString(" intercalary keys × 3 rules. Regenerate with:\n")
	b.WriteString("   UPDATE_ANSWER_LADDER=1 go test ./internal/widgets/calendar_block/ */\n")
	for _, k := range answerLadderKeys() {
		fmt.Fprintf(&b,
			".cal-block-host .block:has(.daypick[data-day-pick=\"%s\"]:checked) .lrow:not([data-lday=\"%s\"]) { display: none }\n",
			k, k)
		fmt.Fprintf(&b,
			".cal-block-host .block:has(.daypick[data-day-pick=\"%s\"]:checked) .lhead .lctx[data-lday=\"%s\"],\n"+
				".cal-block-host .block:has(.daypick[data-day-pick=\"%s\"]:checked) .lzero.lday[data-lday=\"%s\"] { display: revert }\n",
			k, k, k, k)
		fmt.Fprintf(&b,
			".cal-block-host .block:has(.grid [data-day-ord=\"%s\"]:is(:hover, :focus-within)) .lrows .lrow[data-lday=\"%s\"],\n"+
				".cal-block-host .block:has(.lrows .lrow[data-lday=\"%s\"]:is(:hover, :focus-within)) .grid [data-day-ord=\"%s\"] { --answer: 1 }\n",
			k, k, k, k)
	}
	b.WriteString(answerLadderEnd)
	return b.String()
}

// cellAxis is the cell's DATUM HUE for the ANSWER wash: the first mark's axis,
// normalised through the same whitelist every other axis value passes.
//
// It is the tick rule's existing derivation (tickDays reads c.Marks[0].Axis)
// applied one surface up, not a new fact — DayCell has no axis field and does
// not need one. A cell with no marks returns "" and carries no --axis at all,
// so the ANSWER wash falls back to the neutral structural rule rather than
// borrowing a hue from a day that has no events.
func cellAxis(c DayCell) string {
	if len(c.Marks) == 0 {
		return ""
	}
	return axisToken(c.Marks[0].Axis)
}

// ── Zone C · the docked Ledger's content ────────────────────────────────────
//
// THE LEDGER IS REASSEMBLED FROM THE CELLS THE GRID ALREADY DRAWS. There is no
// Ledger.Rows list on the pin and there deliberately is not going to be (r52
// §5): a parallel row list re-opens the two-computations defect that
// block_projection.go exists to kill. Walking Month.Rows[].Cells[] plus
// Month.Intercalary[].Marks STRUCTURALLY GUARANTEES that the Ledger and the
// grid come from one viewer-filtered pass, which makes the count-oracle
// discipline unbreakable instead of merely re-asserted — the head count, the
// foot count and the day-cell total are three readings of one number.
//
// It is safe because DayCell.Marks is the FULL viewer-visible list (data.go):
// blockCapMarks returns marks unchanged and only computes the fold count, so
// nothing the Ledger should list is missing from the cells.
//
// THE LEDGER IS NEVER HORIZON-FILTERED. visFor() applies no fog filter and the
// signed v4-plus-list.png shows the GM Ledger listing day 26 while the grid
// blanks days 22-30 — "the Ledger already carries the GM's working list for
// fogged days, so the grid never has to" is the argument that let the week-scale
// fog reversal be withdrawn. DayCell.Fogged is permanently false in wave 1, so
// nothing bites today; W-F adds the GRID-side split only.

// ledgerLine is one assembled Ledger row: a mark, plus the day it sits on and
// that day's two keys.
type ledgerLine struct {
	// Ord is the LADDER key ("3" / "i1") — what the generated sheet filters on.
	Ord string
	// Key is the ANSWER key (dayKey / intercalaryKey) — what a partner surface
	// elsewhere in the product matches on.
	Key  string
	Day  int
	Mark Mark
}

// ledgerContext is one day's head line and, when the day is empty, its own
// "nothing here" line. Both are rendered for EVERY selectable day and revealed
// by the ladder, because CSS cannot compute "3 Deepwinter · 1 event".
type ledgerContext struct {
	Ord   string
	Label string // "3 Deepwinter" / the intercalary day's own name
	Text  string // the head line: Label + " · N events"
	Empty bool   // this day carries no marks, so it needs a .lzero of its own
}

// ledgerView is everything Zone C draws, computed once per render.
type ledgerView struct {
	Lines []ledgerLine
	Ctx   []ledgerContext
	// Head is the UNSELECTED head line: "<Month> · N events".
	//
	// The scope chip the signed head also carries (opts.scopeLabel, cv4:1742)
	// is NOT printed: BlockData has no scope field, and inventing one is the
	// thing this arc refuses. Booked, not closed (.ai/todo.md).
	Head string
	// Zero is true when the whole month carries no marks at all.
	Zero bool
}

// newLedgerView assembles Zone C. One walk, no second pass over anything.
func newLedgerView(d BlockData) ledgerView {
	cells := make(map[int]DayCell, d.Month.Days)
	for _, r := range d.Month.Rows {
		for _, c := range r.Cells {
			if c.Day > 0 {
				cells[c.Day] = c
			}
		}
	}

	v := ledgerView{Lines: make([]ledgerLine, 0, 16)}
	perDay := make(map[string]int, d.Month.Days+len(d.Month.Intercalary))

	// Ordinal order, which is the Ledger's own promise: it "lists by ordinal
	// day, so it works before eras are defined".
	for day := 1; day <= d.Month.Days; day++ {
		c, ok := cells[day]
		if !ok {
			continue
		}
		ord := dayOrdKey(day)
		perDay[ord] = len(c.Marks)
		for _, m := range c.Marks {
			v.Lines = append(v.Lines, ledgerLine{
				Ord: ord, Key: dayKey(d.CalendarSlug, day), Day: day, Mark: m,
			})
		}
	}
	for _, ic := range d.Month.Intercalary {
		ord := intercalaryOrdKey(ic.Day)
		perDay[ord] = len(ic.Marks)
		for _, m := range ic.Marks {
			v.Lines = append(v.Lines, ledgerLine{
				Ord: ord, Key: intercalaryKey(d.CalendarSlug, ic.Day), Day: ic.Day, Mark: m,
			})
		}
	}

	v.Zero = len(v.Lines) == 0
	v.Head = d.Month.Name + " · " + eventCountLabel(len(v.Lines))

	// One context per SELECTABLE day. Past the ladder's bound a day has no
	// control, so a context for it could never be revealed — emitting one would
	// be dead markup on every render of a long month.
	for day := 1; day <= d.Month.Days; day++ {
		if _, ok := cells[day]; !ok || !dayAnswers(day) {
			continue
		}
		ord := dayOrdKey(day)
		label := intText(day) + " " + d.Month.Name
		v.Ctx = append(v.Ctx, ledgerContext{
			Ord: ord, Label: label,
			Text:  label + " · " + eventCountLabel(perDay[ord]),
			Empty: perDay[ord] == 0,
		})
	}
	for _, ic := range d.Month.Intercalary {
		if !intercalaryAnswers(ic.Day) {
			continue
		}
		ord := intercalaryOrdKey(ic.Day)
		label := ic.Name
		if strings.TrimSpace(label) == "" {
			label = intText(ic.Day) + " " + d.Month.Name
		}
		v.Ctx = append(v.Ctx, ledgerContext{
			Ord: ord, Label: label,
			Text:  label + " · " + eventCountLabel(perDay[ord]),
			Empty: perDay[ord] == 0,
		})
	}
	return v
}

// ledgerZeroAll is the UNSELECTED empty state. Its second sentence is
// load-bearing and not decoration: it is the reason the Ledger renders in the
// FAULT case at all. A calendar that cannot resolve a date prints its fault
// where the date would go (BlockData.Fault) and STILL lists its events, because
// listing by ordinal day needs no era, no epoch and no reckoning.
func ledgerZeroAll(d BlockData) string {
	return "No events in " + d.Month.Name + "."
}

// ledgerZeroDay is the SELECTED empty state — a different string from
// ledgerZeroAll, and both are signed. "Nothing on 3 Deepwinter" names the day
// the viewer chose; "No events in Deepwinter" names the month they are looking
// at. Printing the month's string for a chosen day would say the month is empty
// when it is not.
func ledgerZeroDay(c ledgerContext) string { return "Nothing on " + c.Label + "." }

// ledgerRowClass stamps the row's tie state so the tie toggle re-inks the
// Ledger exactly as it re-inks the grid — one flip, both zones.
//
// THE ROW DOES NOT CARRY `.lday`, AND THAT IS LOAD-BEARING. `.lday` is the
// token for a per-day surface that is display:none AT REST and revealed by the
// ladder when its day is chosen — the head's context line and the per-day
// empty state, and only those two. The row is the opposite kind of surface: it
// is visible at rest and HIDDEN by the ladder's filter, which selects it by its
// `data-lday` attribute and never by a class.
//
// Carrying both was the fix round's blocking defect. The reveal rule matched
// the row, `display: block` beat `.lrow { display: flex }` on specificity and
// source order, and every listed row for the chosen day collapsed into a stack
// with the time overprinting the numeral — invisibly to every height assertion,
// because .lrow's height is fixed. The ladder is now scoped to its two surfaces
// as well (answerLadderCSS rule 2); this is the second of the two locks and the
// reason a future hand cannot re-open the collision by "tidying" the selector.
func ledgerRowClass(m Mark) string { return "lrow" + tieClass(m) }

// ledgerShowsGoldRail reports whether a row draws the gold GM rail and the `GM`
// badge.
//
// IT SPLITS THE SAME WAY THE GRID'S DOGEAR AND DIAMOND DO, and that is the
// whole point: Audience.Restricted is the DISCRIMINATOR, not a synonym for
// "hidden". dm_only (Restricted false) is the gold notch in the grid and the
// gold rail here; a visibility_rules restriction (Restricted true) is the
// diamond in the grid and the audience chip alone here. A Ledger that drew the
// gold rail on a restricted-audience row would be the identical defect one
// layer down from the one SEAM-P5 stage 4 fixed.
func ledgerShowsGoldRail(m Mark) bool {
	return m.Audience != nil && !m.Audience.Restricted
}

// ledgerMetaLine is the row's meta line.
//
// WAVE 2 PRINTS THE TYPE ALONE. The signed line is "<type lowercased> ·
// <Owner>", but Event carries no owner/creator surface in the projection and
// resolving a display name for a user id implies caching this wave has no
// reason to build (CTS-5, SIGNED: omit). It drops the absent segment rather
// than printing "quest · " with a dangling separator — the blockSyncStrings
// idiom. `· RSVP 3/5` is not printed either: RSVP SURFACES are W-G, and the
// Bench's RSVP panel already carries the `needs backend` chip for that reason.
//
// Empty means the row emits no .mt element at all, not an empty one.
func ledgerMetaLine(m Mark) string {
	return strings.ToLower(strings.TrimSpace(m.AxisLabel))
}

// ledgerTimeClass picks the row's time treatment, and it is a per-CALENDAR
// decision rather than a per-event one (L15, design notes §9 dev 5): a
// zone-labelled real-world time printed on an in-world calendar would
// contradict L15's own rule. BlockData.IsRealWorld is the one fact that decides
// it, which is exactly why r52 added no per-mark real-world flag.
func ledgerTimeClass(d BlockData) string {
	if d.IsRealWorld {
		return "tm"
	}
	return "tm mono"
}

// ── the counts the Block puts on screen ─────────────────────────────────────

// hiddenBadgeText is the signed "<N> hidden" chip (cv4:1650-1651), which
// nameplate.templ used to record as absent "because the pinned struct has
// nowhere to put the number, not because it was rejected". r52 gave it one.
//
// GM-ONLY BY CONSTRUCTION, TWICE. The producer sets ViewerContext.HiddenCount
// only when IsGM and leaves it zero for every other viewer at the source (r52
// rule 1). This gate is NOT a substitute for that one — it is a second lock on
// the same door, because the one thing that must never happen is a player
// receiving this number in any form, including a zero. Permission is ABSENCE:
// a player has no second number to difference it against because a player has
// no number at all.
//
// `.badge.gm` carries exactly two strings in the signed set — "<N> hidden" and
// "GM". It is not extended.
func hiddenBadgeText(d BlockData) string {
	if !d.Viewer.IsGM || d.Viewer.HiddenCount <= 0 {
		return ""
	}
	return intText(d.Viewer.HiddenCount) + " hidden"
}

// blockFootCount is the mini / sub-mini foot's number, and it is NOT always the
// month's total.
//
// DELIBERATE, PRE-AUTHORISED DIVERGENCE FROM THE MINI STILLS (dispatch §6). The
// mockup's foot uses visFor(m).length and ignores the scope the entity sidebar
// passes (cv4:2410-2411), so the Hollowmere-scoped 280px sidebar prints "14
// events" while its own Attributes card, two inches above, prints "Ties 4".
// That is the tie-count oracle's shape one size down, and it is visible in
// v4-entity-whole-light.png.
//
// So: when the Block is entity-hosted AND tie-scoped, the foot states the TIED
// count and says the word "tied". The tie toggle is display:none at that size,
// so an unqualified number beside an entity reads as the entity's — and at that
// size it had better be. Both numbers are already on ViewerContext; this needs
// no new pin field and no second pass.
func blockFootCount(d BlockData) string {
	if d.Viewer.HostEntity != "" && d.Viewer.TieMode == "tied" {
		return eventCountLabel(d.Viewer.TiedCount) + " tied"
	}
	return eventCountLabel(totalMarks(d))
}

// ── Zone D · the Shelf ──────────────────────────────────────────────────────
//
// THE SHELF READS. It authors nothing and it grows no configuration surface of
// its own — contract §8 item 2 (L5) says the celestial surface is "a dedicated
// filtered Almanac list in the shelf widget. No fifth authoring surface", and
// that clause is a bound rather than decoration.
//
// THREE PANELS, ONE RADIO GROUP, ZERO JAVASCRIPT ([S7], SIGNED). A bound Block
// renders under context.Background() (entity_calendar_block.go), so the
// mockup's `?alm=` query param is not reachable, and per-viewer persistence is
// W-F's store, which does not exist. The tabs are therefore the same CSS-only
// radio mechanism the tie toggle proved (nameplate.templ, TestCSS_TieToggle-
// FlipsInkWithoutJS): the server renders the default pressed state, the
// stylesheet does the rest, and W-F later swaps the DEFAULT's source for a
// stored preference and touches nothing else.

// shelfPickGroupName is the radio group shared by this Block's three Shelf
// tabs, and shelfPickInputID one tab's DOM id.
//
// Pure functions of the data, for the third time in this package and for the
// same two reasons (see tieGroupName): the Bench composes four Blocks on one
// page and two of them sharing a radio name would fight over one piece of
// state, while the SAME Block re-rendered by an HTMX binding swap must keep the
// name or the viewer's chosen tab is lost in the swap.
func shelfPickGroupName(d BlockData) string {
	return "shelf-" + domToken(d.CalendarSlug) + "-" + domToken(d.Viewer.HostEntity)
}

func shelfPickInputID(d BlockData, key string) string {
	return shelfPickGroupName(d) + "-" + domToken(key)
}

// The Shelf tab keys, declared once. The stylesheet writes one rule per key
// against these exact strings. The Almanac's key is added with its panel.
const (
	shelfTabUpcoming = "upcoming"
	shelfTabFilters  = "filters"
)

// shelfDefaultTab is the SERVER-RENDERED pressed tab, and it is the one thing
// W-F has to change when the per-viewer store exists.
//
// The signed default is `SKY_ON() && m.moons` (cv4:1777-1783) — the Almanac
// leads when there is a sky to lead with, and Upcoming otherwise. This stage
// ships the Upcoming half; the Almanac's term arrives with the Almanac panel,
// in one place, so the tab that is pressed and the panel that exists can never
// disagree.
func shelfDefaultTab(d BlockData) string {
	return shelfTabUpcoming
}

// shelfShowsFilters gates the Filters tab on the viewer being a GM.
//
// [S10], SIGNED: a 2-tab Shelf for players and a 3-tab Shelf for the GM. It is
// the 2026-07-27 needs-backend-audience ruling applied one level down — "for a
// player the zone simply does not appear" — and it is ABSENCE rather than a
// disabled control. The Filters panel is a bare `needs backend` chip ([S2]) and
// that chip must never render to a player; gating the chip alone would leave a
// player a tab that opens on nothing, which is worse than either.
//
// It is worth naming as what it is: the FIRST per-role difference inside a
// chrome strip rather than inside content. Everywhere else on the Block,
// permission is absence of DATA.
func shelfShowsFilters(d BlockData) bool { return d.Viewer.IsGM }

// shelfUpcoming is Zone D's Upcoming panel, assembled from the SAME
// ledgerView the Ledger draws.
//
// IT REUSES THE LEDGER'S ROW PRIMITIVE VERBATIM (cv4:1794, :1798 — the signed
// panel calls ledgerRow(e, 'full', i)). There is no second row implementation
// and no second walk: newLedgerView already produced every line from the one
// viewer-filtered pass, so the Shelf's counts cannot disagree with the Ledger's
// head, the Block's foot or the day-cell total. A parallel list here would
// re-open the two-computations defect for the fourth time.
//
// MONTH-SCOPED, EXACTLY AS DRAWN ([S1], SIGNED). The panel is Today plus up to
// four later days of the SAME month on ONE calendar. The cross-calendar index
// exists — BlockService.UpcomingAcrossCalendars — but it is the BENCH's NEXT UP
// surface, and a second one inside the Block is the duplicate-surface decay the
// proportion rule exists to stop. It also keeps the Block's data self-contained
// under context.Background(), which is what lets a Block be embedded anywhere.
//
// THE FOUR-ROW CAP IS NORMATIVE ([S1]). `.sp2` is a fixed 132px with its own
// scroller; the cap is what keeps the scroller from becoming the panel.
type shelfUpcomingView struct {
	// Today is the rows on TodayDay. Empty is a real state and gets its own
	// line, because "no events today" and "today is not in this month" are
	// different claims — the same split ledgerZeroAll/ledgerZeroDay makes.
	Today []ledgerLine
	// Later is the next LaterCap days' rows, in ordinal order.
	Later []ledgerLine
	// TodayInMonth is false when the calendar's current day falls outside the
	// rendered month, which is the ordinary case for any month but this one.
	TodayInMonth bool
	// Truncated is true when the cap dropped rows, so the panel can say so
	// rather than silently ending. A list that stops without saying it stopped
	// is the same class of omission as a count with no denominator.
	Truncated bool
}

// shelfUpcomingCap is the signed `.slice(0, 4)` (cv4:1798), normative per [S1].
const shelfUpcomingCap = 4

func newShelfUpcoming(d BlockData) shelfUpcomingView {
	v := shelfUpcomingView{TodayInMonth: d.Month.TodayDay > 0}
	for _, l := range newLedgerView(d).Lines {
		// INTERCALARY DAYS ARE NOT IN THIS LIST, and that is arithmetic rather
		// than a judgement: an intercalary day's ordinal is its own (Midwinter
		// 1 is not Deepwinter 1), so comparing it against TodayDay would order
		// it against a different scale. They keep their place in the Ledger,
		// which lists by ordinal day within each namespace.
		if strings.HasPrefix(l.Ord, "i") {
			continue
		}
		switch {
		case v.TodayInMonth && l.Day == d.Month.TodayDay:
			v.Today = append(v.Today, l)
		case l.Day > d.Month.TodayDay:
			if len(v.Later) < shelfUpcomingCap {
				v.Later = append(v.Later, l)
			} else {
				v.Truncated = true
			}
		}
	}
	return v
}

// shelfTodayZero is the Today band's empty line. Two strings for two claims:
// the month HAS a today and nothing is on it, or the month does not contain
// today at all — printing "Nothing today" for the second would assert the
// viewer is looking at the current month when they are not.
func shelfTodayZero(d BlockData, v shelfUpcomingView) string {
	if !v.TodayInMonth {
		return "Today is not in " + d.Month.Name + "."
	}
	return "Nothing today."
}

// shelfLaterZero is the This month band's empty line — the month has no events
// after the anchor day. It names the month for the same reason ledgerZeroAll
// does: the claim is about the month, not about a day.
func shelfLaterZero(d BlockData) string {
	return "Nothing further in " + d.Month.Name + "."
}

// shelfMoreText states what the four-row cap dropped. The cap is normative, so
// the honest form is to say the list is capped rather than to end silently —
// the Ledger, one zone up, lists every row and is one scroll away.
func shelfMoreText() string {
	return "First " + strconv.Itoa(shelfUpcomingCap) + " shown — the Ledger lists the whole month."
}
