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


func boolAttr(b bool) string {
	if b {
		return "true"
	}
	return "false"
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
