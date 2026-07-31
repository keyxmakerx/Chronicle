package calendar

// daycard_test.go — C-CALV4-DAYCARD (calendar-v4 round 2, slice R2-2a).
//
// THE PAYLOAD IS THE LEAK SURFACE, so it is the thing this file pins. Three
// classes of assertion, and each one exists because the corresponding failure
// would be invisible in a screenshot:
//
//  1. THE FIELD SET. [DC-1] SIGNED fixes the payload at the Ledger row's own
//     fields and NOTHING more. A payload that grew a description body would put
//     every event's prose into every viewer's DOM for a card that never
//     displays it — and the card would look identical.
//  2. THE KEY NAMESPACES. The card is opened from a cell by matching
//     `data-day`, and its Ledger door finds the day's radio by `data-day-pick`.
//     Both keys are minted by UNEXPORTED helpers in
//     internal/widgets/calendar_block, which this slice may not open, so they
//     are mirrored in daycard.go — and mirrored code drifts. The assertion is
//     therefore against the RENDERED Block, not against the mirror.
//  3. THE MOUNT. The card is emitted only when there is a payload to open it
//     on, and a player's Bench carries the card but no authoring DOM.
//
// The agreement law itself ([DC-2]) lives in block_count_oracle_test.go, JOINED
// to the count oracle rather than forked into a harness of its own.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// dayCardFixtureBlock projects the signed oracle month for one viewer, with the
// Bench's own layer set, so the payload under test is built from the DOM the
// Bench actually renders.
func dayCardFixtureBlock(t *testing.T, v BlockViewer) calblock.BlockData {
	t.Helper()
	cal, events := oracleLedgerFixture()
	d := projectBlock(BlockProjectionInput{
		Calendar: cal, Events: blockCopyEvents(events),
		Viewer: v, MonthIndex: 0, Year: 1523,
	})
	d.Layers = benchBlockLayers(blockLayerPrefs{})
	return d
}

// --- 1. the payload law -----------------------------------------------------

// TestDayCard_PayloadCarriesTheLedgerRowFieldSetAndNothingMore is [DC-1]'s
// enforcement, and it asserts on the SHIPPED JSON KEYS rather than on the Go
// struct: a field added with a `json:"-"` tag would pass a struct assertion and
// a field added to the struct is exactly how this grows.
//
// The forbidden list is named explicitly because each entry is a real
// temptation with a real cost: description/description_html are the prose the
// editor writes and the card never prints; visibility_rules is the audience
// whitelist itself; recurrence is the editor's own model. All three come from
// the editor's route under the editor's role floor, never from a page attribute
// every viewer's browser receives.
func TestDayCard_PayloadCarriesTheLedgerRowFieldSetAndNothingMore(t *testing.T) {
	d := dayCardFixtureBlock(t, BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner})
	raw := dayCardPayloadJSON(&BenchBlock{Data: d})
	if raw == "" {
		t.Fatal("the GM fixture produced no payload at all")
	}

	var decoded struct {
		Calendars []struct {
			ID           string            `json:"id"`
			Slug         string            `json:"slug"`
			LedgerDocked bool              `json:"ledgerDocked"`
			Days         []json.RawMessage `json:"days"`
		} `json:"calendars"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if len(decoded.Calendars) != 1 {
		t.Fatalf("want one calendar entry, got %d", len(decoded.Calendars))
	}

	dayKeys := map[string]bool{}
	eventKeys := map[string]bool{}
	events := 0
	for _, rawDay := range decoded.Calendars[0].Days {
		var day map[string]json.RawMessage
		if err := json.Unmarshal(rawDay, &day); err != nil {
			t.Fatalf("day is not an object: %v", err)
		}
		for k := range day {
			dayKeys[k] = true
		}
		var evs []map[string]json.RawMessage
		if err := json.Unmarshal(day["events"], &evs); err != nil {
			t.Fatalf("events is not an array: %v", err)
		}
		for _, ev := range evs {
			events++
			for k := range ev {
				eventKeys[k] = true
			}
		}
	}
	if events == 0 {
		t.Fatal("the fixture produced no events; every assertion below would be vacuous")
	}

	// THE STRUCT'S OWN INVENTORY, from a fully-populated value: `omitempty`
	// means the fixture's own rows can never enumerate the whole shape (the
	// signed month declares no times), so the shape is read off the type and
	// the fixture is then checked to stay inside it. Both halves are needed —
	// the first catches a field ADDED, the second catches a field LEAKING.
	full, err := json.Marshal(dayCardEvent{
		ID: "e", Title: "t", Time: "1:00", Axis: "var(--own-1)",
		Pattern: "p1", Glyph: "◆", Gold: true, Audience: "GM only",
	})
	if err != nil {
		t.Fatalf("marshal a populated row: %v", err)
	}
	var fullKeys map[string]json.RawMessage
	if err := json.Unmarshal(full, &fullKeys); err != nil {
		t.Fatalf("decode a populated row: %v", err)
	}
	declared := map[string]bool{}
	for k := range fullKeys {
		declared[k] = true
	}

	wantDay := []string{"day", "events", "key", "label", "ord", "weekday"}
	if got := sortedKeys(dayKeys); !equalStrings(got, wantDay) {
		t.Errorf("day object keys = %v, want exactly %v", got, wantDay)
	}
	wantEvent := []string{"audience", "axis", "glyph", "gold", "id", "pattern", "time", "title"}
	if got := sortedKeys(declared); !equalStrings(got, wantEvent) {
		t.Errorf("dayCardEvent declares JSON keys %v, want exactly %v — [DC-1] SIGNED "+
			"fixes the payload at the Ledger row's field set and NOTHING more", got, wantEvent)
	}
	for _, k := range sortedKeys(eventKeys) {
		if !declared[k] {
			t.Errorf("a rendered row carries the key %q, which the type does not declare", k)
		}
	}

	for _, forbidden := range []string{
		"description", "description_html", "descriptionHtml",
		"visibility_rules", "visibilityRules", "visibility",
		"recurrence", "recurrence_type", "is_recurring", "isRecurring",
		"entity_id", "entityId", "tier", "all_day", "created_by",
	} {
		if strings.Contains(raw, `"`+forbidden+`"`) {
			t.Errorf("the payload carries %q — it is the EDITOR's field, it is "+
				"leak-sensitive, and the card never prints it", forbidden)
		}
	}
}

// TestDayCard_APlayerPayloadCarriesNoGMMarkInAnyForm. The producer leaves
// AudienceMark nil for a viewer who is not entitled to it, so `gold` and
// `audience` are absent for a player BY CONSTRUCTION — this asserts the
// construction rather than trusting it, because the card is the first v4
// surface where a player opens something adjacent to authoring.
func TestDayCard_APlayerPayloadCarriesNoGMMarkInAnyForm(t *testing.T) {
	gm := dayCardPayloadJSON(&BenchBlock{
		Data: dayCardFixtureBlock(t, BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}),
	})
	if !strings.Contains(gm, `"gold":true`) {
		t.Fatal("the GM payload carries no gold rail at all; the player assertion below " +
			"would be vacuous")
	}
	for _, name := range []string{"u-nissa", "u-bryn"} {
		raw := dayCardPayloadJSON(&BenchBlock{
			Data: dayCardFixtureBlock(t, BlockViewer{UserID: name, Role: permissions.RolePlayer}),
		})
		for _, bad := range []string{`"gold"`, `"audience"`, "GM only", "Restricted"} {
			if strings.Contains(raw, bad) {
				t.Errorf("%s's payload contains %q — permission is ABSENCE, and a "+
					"player has no second number to difference it against", name, bad)
			}
		}
	}
}

// --- 2. the mirrored key namespaces -----------------------------------------

var dayCardAttrRe = regexp.MustCompile(`data-day="([^"]+)"`)
var dayCardOrdRe = regexp.MustCompile(`data-day-ord="([^"]+)"`)

// TestDayCard_KeysAgreeWithTheRenderedBlock is the mirror's guard.
//
// daycard.go re-implements dayKey / intercalaryKey / dayOrdKey /
// intercalaryOrdKey because they are unexported and this slice opens NO file in
// internal/widgets/calendar_block. A mirror that drifted would produce a card
// that never opens — the failure would look exactly like the operator's
// original complaint, which is why it is pinned against the RENDERED Block's
// own attributes and not against the mirror's source.
func TestDayCard_KeysAgreeWithTheRenderedBlock(t *testing.T) {
	d := dayCardFixtureBlock(t, BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner})
	body := seamRenderBlockData(t, d)

	rendered := map[string]bool{}
	for _, m := range dayCardAttrRe.FindAllStringSubmatch(body, -1) {
		rendered[m[1]] = true
	}
	renderedOrd := map[string]bool{}
	for _, m := range dayCardOrdRe.FindAllStringSubmatch(body, -1) {
		renderedOrd[m[1]] = true
	}
	if len(rendered) == 0 || len(renderedOrd) == 0 {
		t.Fatal("the rendered Block carries no day keys at all")
	}

	card := buildDayCardCalendar(d)
	if len(card.Days) == 0 {
		t.Fatal("the payload carries no days")
	}
	for _, day := range card.Days {
		if !rendered[day.Key] {
			t.Errorf("the payload's day key %q appears on no node of the rendered Block — "+
				"the mirrored dayKey namespace has drifted and the card would never open",
				day.Key)
		}
		if !renderedOrd[day.Ord] {
			t.Errorf("the payload's ladder key %q appears on no node of the rendered Block — "+
				"the mirrored dayOrdKey namespace has drifted and the `Open in the Ledger` "+
				"door would find no radio", day.Ord)
		}
	}
}

// TestDayCard_LedgerDockedMirrorsTheWidgetsOwnPredicate. Absence has TWO
// causes and the card's Ledger door depends on telling them apart, so both are
// exercised: a host that never docked the zone (no `ledger` layer) and a viewer
// whose host docked it hidden (LedgerHidden).
func TestDayCard_LedgerDockedMirrorsTheWidgetsOwnPredicate(t *testing.T) {
	base := dayCardFixtureBlock(t, BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner})
	if !dayCardLedgerDocked(base) {
		t.Fatal("the Bench's own layer set should dock the Ledger")
	}

	off := base
	off.Layers = calblock.LayerState{Enabled: []string{"moons", "eras", "weeknums", "shelf"}}
	if dayCardLedgerDocked(off) {
		t.Error("a viewer who switched the `ledger` layer off has no docked Ledger")
	}
	if buildDayCardCalendar(off).LedgerDocked {
		t.Error("the payload must carry the layer-off state, so the module never offers " +
			"a door onto a column that is not on the page")
	}

	hidden := base
	hidden.Ledger = calblock.LedgerStub{Hidden: true}
	if dayCardLedgerDocked(hidden) {
		t.Error("a host that docked the zone HIDDEN has no docked Ledger either")
	}

	// AND THE CARD STILL OPENS WITH THE LEDGER OFF. This is the case the
	// operator hit hardest — dayPick emits no radio and no label at all
	// (instrument.templ:213), so the click is not quiet, it is absent — and it
	// is the case a card built from the rendered Ledger DOM could not serve.
	if len(buildDayCardCalendar(off).Days) == 0 {
		t.Error("with the Ledger switched off the card is the ONLY answer, and it must " +
			"still carry every day")
	}
}

// TestDayCard_WeekdayComesFromTheCalendarAndNotFromASeven. The fixture's month
// is a TEN-day week. A `% 7` anywhere in the derivation would name the wrong
// weekday on every calendar in the product that is not Gregorian, and the
// failure reads as a cosmetic typo rather than as a broken rule.
func TestDayCard_WeekdayComesFromTheCalendarAndNotFromASeven(t *testing.T) {
	d := dayCardFixtureBlock(t, BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner})
	if d.Month.WeekLen != 10 {
		t.Fatalf("the fixture's week is %d days; this assertion needs the ten-day one",
			d.Month.WeekLen)
	}
	card := buildDayCardCalendar(d)
	byDay := map[int]string{}
	for _, day := range card.Days {
		byDay[day.Day] = day.Weekday
	}
	if byDay[1] == "" {
		t.Fatal("day 1 has no weekday; the fixture declares none and the assertion is vacuous")
	}
	if byDay[1] != byDay[11] || byDay[1] != byDay[21] {
		t.Errorf("days 1/11/21 of a ten-day week are the same weekday; got %q/%q/%q",
			byDay[1], byDay[11], byDay[21])
	}
	if byDay[1] == byDay[8] {
		t.Errorf("day 8 shares day 1's weekday (%q) — that is a seven-day derivation on a "+
			"ten-day calendar", byDay[1])
	}
}

// TestDayCard_TheStyleChannelsAreNormalisedAtTheProducer. The card's rows are
// built in JS and the axis crosses a JSON boundary before it is written into a
// custom property. Normalising here means the module never has to be trusted
// with a sanitiser of its own.
func TestDayCard_TheStyleChannelsAreNormalisedAtTheProducer(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"var(--own-3)", "var(--own-3)"},
		{"#3b82f6", "#3b82f6"},
		{"red; background:url(x)", calblock.AxisFallback},
		{"", calblock.AxisFallback},
	} {
		if got := dayCardAxis(tc.in); got != tc.want {
			t.Errorf("dayCardAxis(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	for _, tc := range []struct{ in, want string }{
		{"p3", "p3"}, {"p8", "p8"}, {"p9", "p1"}, {"", "p1"}, {"p3 evil", "p1"},
	} {
		if got := dayCardPattern(tc.in); got != tc.want {
			t.Errorf("dayCardPattern(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- 3. the mount -----------------------------------------------------------

// TestDayCard_MountIsPresentForEveryRoleAndAbsentWithoutAPayload.
//
// READING A DAY IS A PLAYER AFFORDANCE ([DC-9] SIGNED): the card mounts for
// every role. What a player does not get is anything to author with, and that
// is asserted separately below. A Bench with no Block gets NO scaffold, no
// stylesheet and no script — orphan DOM keyed to invokers that do not exist is
// the same thing bench.templ's header refuses for popovers().
func TestDayCard_MountIsPresentForEveryRoleAndAbsentWithoutAPayload(t *testing.T) {
	for _, tc := range []struct {
		name string
		data BenchData
	}{
		{"GM", benchFxData(true, true)},
		{"player", benchFxData(false, false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.data.DayCardJSON == "" {
				t.Fatal("the fixture built no payload; the mount assertions would be vacuous")
			}
			body := renderBench(t, tc.data)
			for _, want := range []string{
				`data-cal-daycard-payload="`,
				`data-cal-daycard`,
				`popover="manual"`,
				`/static/css/calendar-daycard.css`,
				`/static/plugins/calendar/js/calendar_daycard.js`,
				`data-dc-rows`, `data-dc-empty`, `data-dc-foot`, `data-dc-box`,
			} {
				if !strings.Contains(body, want) {
					t.Errorf("the Bench does not mount the day card: missing %q", want)
				}
			}
		})
	}

	bare := benchFxData(true, true)
	bare.Primary, bare.RealWorld, bare.DayCardJSON = nil, nil, ""
	body := renderBench(t, bare)
	for _, bad := range []string{
		`data-cal-daycard`, `/static/css/calendar-daycard.css`,
		`/static/plugins/calendar/js/calendar_daycard.js`,
	} {
		if strings.Contains(body, bad) {
			t.Errorf("a Bench with no Block still emits %q — there is no day to open on", bad)
		}
	}
}

// TestDayCard_APlayerBenchCarriesNoAuthoringDOMAndNoHonestyChip.
//
// TWO RULES, ONE ASSERTION, AND THEY ARE DIFFERENT RULES. `needs backend` marks
// data that does not exist FOR ANYBODY and is a visible chip that never reaches
// a player (decisions/2026-07-27-needs-backend-audience.md); permission is
// ABSENCE and hides data that exists from a viewer not entitled to it. The chip
// is what is counted here.
//
// THE LITERAL STRING IS NOT ZERO ON THIS PAGE AND SAYING SO IS THE POINT. The
// Bench's own .caption explains the chip vocabulary in prose — *"Tiles marked
// needs backend name a gap in what Chronicle can compute"* — to every role,
// since wave 1. That is the surface documenting its own honesty states, not an
// honesty state being rendered, so the assertion is on the CHIP (`badge need`)
// and the caption is excluded by name rather than by silence.
func TestDayCard_APlayerBenchCarriesNoAuthoringDOMAndNoHonestyChip(t *testing.T) {
	player := renderBench(t, benchFxData(false, false))

	if n := strings.Count(player, `class="badge need"`); n != 0 {
		t.Errorf("a player's Bench renders %d `needs backend` chips; a chip never reaches "+
			"a player", n)
	}

	// The card's own DOM carries no authoring affordance in this stage at all —
	// the editor and its `+ New event` door are stage 2 — so the absence is
	// asserted on the markers a later stage will introduce, and this assertion
	// is what stops them appearing on a player's page.
	for _, bad := range []string{
		"data-dc-new", "data-cal-dayeditor", "New event", "needs backend</span>",
	} {
		if strings.Contains(player, bad) {
			t.Errorf("a player's Bench contains %q — a control the viewer may not use is "+
				"NOT RENDERED ([DC-9] SIGNED: markup-level, never CSS, never JS)", bad)
		}
	}

	// …and the caption's explanatory sentence IS still there, so the assertion
	// above is proving an absence of chips rather than an absence of the page.
	if !strings.Contains(player, "Tiles marked") {
		t.Error("the Bench's caption is gone; the chip-count assertion above no longer " +
			"distinguishes a chip from a page that stopped rendering")
	}
}

// --- 4. the sheets ----------------------------------------------------------

func dayCardCSS(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	body, err := os.ReadFile(filepath.Join(root, "static", "css", "calendar-daycard.css"))
	if err != nil {
		t.Fatalf("read calendar-daycard.css: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("calendar-daycard.css is empty")
	}
	return string(body)
}

// TestDayCardCSS_CarriesNoMotionOfItsOwn is the register's MONOPOLY guard,
// reaching one file further than BENCH-R2 could.
//
// [DC-6] SIGNED resolved ownership by the first-lander clause: BENCH-R2 owns
// the register and DAYCARD consumes it. The failure mode that ruling exists to
// prevent is precisely this file — a second sheet, out of reach of
// TestBenchCSS_NoMotionAtAll, quietly growing a second grammar while the Bench
// guard stays green. Laundering a guard is not something this arc does, so the
// new sheet is guarded rather than trusted.
func TestDayCardCSS_CarriesNoMotionOfItsOwn(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(dayCardCSS(t), " ")
	for _, bad := range []string{
		"transition", "animation", "@keyframes", "will-change",
		"@starting-style", "view-transition",
	} {
		if strings.Contains(code, bad) {
			t.Errorf("calendar-daycard.css contains %q — the card's motion belongs in the "+
				"ONE register section of calendar-bench.css ([DC-6] SIGNED); a second sheet "+
				"is laundering", bad)
		}
	}
}

// TestDayCardCSS_EverySelectorIsScoped. This sheet is unlayered and outranks the
// whole layered app cascade, and it deliberately reuses the LEDGER ROW's
// vocabulary — .rail, .nm, .tm, .badge, .gr, .tok — so the two surfaces read
// alike. That makes an unscoped rule here more dangerous than usual, not less.
func TestDayCardCSS_EverySelectorIsScoped(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(dayCardCSS(t), " ")
	for _, line := range strings.Split(code, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || !strings.HasSuffix(l, "{") || strings.HasPrefix(l, "@") || strings.HasPrefix(l, "}") {
			continue
		}
		sel := strings.TrimSpace(strings.TrimSuffix(l, "{"))
		if sel != "" && !strings.Contains(sel, ".cal-daycard") {
			t.Errorf("unscoped selector in calendar-daycard.css: %q", sel)
		}
	}
}

// TestDayCardCSS_DefinesWhatTheModuleNames closes the #568 gap for this surface:
// every class calendar_daycard.js builds must exist in the sheet, or the card
// renders as unstyled text and every DOM assertion stays green.
func TestDayCardCSS_DefinesWhatTheModuleNames(t *testing.T) {
	code := dayCardCSS(t)
	for _, want := range []string{
		".cal-daycard", ".cal-daycard[data-dc-shown]", ".cal-daycard.dcsheet",
		".cal-daycard .dcbox", ".cal-daycard .dc-h", ".cal-daycard .dc-rows",
		".cal-daycard .dc-row", ".cal-daycard .dc-row .rail",
		".cal-daycard .dc-row .gr", ".cal-daycard .dc-row .tok",
		".cal-daycard .dc-row .nm", ".cal-daycard .dc-row .badge.gm",
		".cal-daycard .dc-row .audchip", ".cal-daycard .dc-row .tm",
		".cal-daycard .dc-empty", ".cal-daycard .dc-f", ".cal-daycard .dc-f .dc-door",
		".cal-daycard .dc-row .rail.p1", ".cal-daycard .dc-row .rail.p8",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("calendar-daycard.css does not define %q", want)
		}
	}
}

// TestDayCardModule_KeepsTheHouseShape asserts the seams that cannot be
// exercised headlessly but must not silently regress (the QA2 source-level
// pattern the drag suite uses).
func TestDayCardModule_KeepsTheHouseShape(t *testing.T) {
	src := readRepoFile(t, "internal/plugins/calendar/static/js/calendar_daycard.js")
	for _, want := range []string{
		"'use strict'",
		"dataset.dcWired",  // the QA2 re-init guard
		"htmx:afterSettle", // boosted-nav re-init
		"htmx:load",
		"showPopover",
		"--disc-close", // the register's own token, read rather than copied
		"prefers-reduced-motion: reduce",
		"input.daypick[data-day-pick=", // the second opener ([DC-4])
	} {
		if !strings.Contains(src, want) {
			t.Errorf("calendar_daycard.js is missing %q", want)
		}
	}
	// THE BOUNDARY, AS A SOURCE-LEVEL REFUSAL. The DOM-level proof is
	// test/js/daycard_block_immutability.test.mjs; this catches the reach
	// before it is written. COMMENTS ARE STRIPPED FIRST: the module's own
	// header NAMES @starting-style and tabindex as the things it refuses, and a
	// guard that fired on the sentence explaining the rule would force the
	// explanation out — which is how a rule stops being written down.
	code := dayCardJSLineComment.ReplaceAllString(src, "")
	for _, bad := range []string{
		"@starting-style", "tabindex", "innerHTML =",
	} {
		if strings.Contains(code, bad) {
			t.Errorf("calendar_daycard.js contains %q — see the module header's boundary "+
				"and [DC-4](b): no injected focusability, no second motion grammar", bad)
		}
	}
}

// --- helpers ----------------------------------------------------------------

// dayCardJSLineComment strips `//` line comments. It is deliberately naive —
// the module has no string literal containing `//`, and asserting that is
// cheaper than shipping a JS tokeniser into a test.
var dayCardJSLineComment = regexp.MustCompile(`(?m)^\s*//.*$`)

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
