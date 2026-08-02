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
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// dayCardFixture projects the signed oracle month for one viewer, with the
// Bench's own layer set, so the payload under test is built from the DOM the
// Bench actually renders.
//
// It returns the projected Block AND the calendar it came from — the pair the
// payload builder takes, because two of the payload's facts (the type palette
// and the intercalary months' adjacency) are calendar-level and BlockData
// deliberately does not carry them.
func dayCardFixture(t *testing.T, v BlockViewer) (calblock.BlockData, *Calendar) {
	t.Helper()
	cal, events := oracleLedgerFixture()
	d := projectBlock(BlockProjectionInput{
		Calendar: cal, Events: blockCopyEvents(events),
		Viewer: v, MonthIndex: 0, Year: 1523,
	})
	d.Layers = benchBlockLayers(blockLayerPrefs{})
	return d, cal
}

// --- 1. the payload law -----------------------------------------------------

// TestDayCard_PayloadCarriesTheLedgerRowFieldSetAndNothingMore is [DC-1]'s
// enforcement, and it asserts on the SHIPPED JSON KEYS — the names that reach
// a viewer's DOM — in three independent ways, because a field added to the
// struct is exactly how this law gets broken:
//
//	the TYPE's declared key set   (reflection; catches a field ADDED, even one
//	                               tagged omitempty that no fixture populates)
//	the RENDERED fixture's keys   (catches a key LEAKING that the type does not
//	                               declare, and keeps the two in agreement)
//	the forbidden-NAME scan       (the named temptations, on the raw string)
//
// The first is round-2's fix-forward (DC2-PAYLOAD-OMITEMPTY): it used to be a
// hand-written literal, which enumerated only the fields whoever wrote it
// remembered, so a ninth `omitempty` field was invisible to it.
//
// The forbidden list is named explicitly because each entry is a real
// temptation with a real cost: description/description_html are the prose the
// editor writes and the card never prints; visibility_rules is the audience
// whitelist itself; recurrence is the editor's own model. All three come from
// the editor's route under the editor's role floor, never from a page attribute
// every viewer's browser receives.
func TestDayCard_PayloadCarriesTheLedgerRowFieldSetAndNothingMore(t *testing.T) {
	d, cal := dayCardFixture(t, BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner})
	raw := dayCardPayloadJSON(dayCardSeed{CanAuthor: true}, dayCardSource{Block: &BenchBlock{Data: d}, Calendar: cal})
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

	// THE STRUCT'S OWN INVENTORY, READ OFF THE TYPE BY REFLECTION — never by
	// marshalling a hand-written literal (DC2-PAYLOAD-OMITEMPTY, round-2
	// fix-forward). EVERY OPTIONAL FIELD HERE IS `omitempty`, so a literal is
	// "fully populated" only for the fields that existed when it was written:
	// a NINTH field added with `omitempty` would simply be absent from that
	// marshal and this comparison would stay green while the payload grew. The
	// literal also had no mechanism forcing anyone to update it. Reflection has
	// no such blind spot — a field cannot be added to the type without
	// appearing here.
	//
	// The fixture cross-check below is the OTHER half and it stays: reflection
	// catches a field ADDED, the fixture catches a rendered key LEAKING that
	// the type does not declare. Neither half subsumes the other, and the
	// forbidden-name list under them catches nothing on its own — a leak under
	// an unlisted name is invisible to it, which is exactly why the two
	// enumerations above it are the real guard.
	declaredEvent := jsonKeySet(t, dayCardEvent{})
	declaredDay := jsonKeySet(t, dayCardDay{})

	wantDay := []string{"day", "events", "key", "label", "month", "ord", "weekday", "year"}
	if got := sortedKeys(declaredDay); !equalStrings(got, wantDay) {
		t.Errorf("dayCardDay declares JSON keys %v, want exactly %v — [DC-1] SIGNED "+
			"fixes the payload at the Ledger row's field set and NOTHING more", got, wantDay)
	}
	for _, k := range sortedKeys(dayKeys) {
		if !declaredDay[k] {
			t.Errorf("a rendered day carries the key %q, which the type does not declare", k)
		}
	}
	wantEvent := []string{"audience", "axis", "glyph", "gold", "id", "pattern", "time", "title"}
	if got := sortedKeys(declaredEvent); !equalStrings(got, wantEvent) {
		t.Errorf("dayCardEvent declares JSON keys %v, want exactly %v — [DC-1] SIGNED "+
			"fixes the payload at the Ledger row's field set and NOTHING more", got, wantEvent)
	}
	for _, k := range sortedKeys(eventKeys) {
		if !declaredEvent[k] {
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

// TestDayCard_PayloadWrapperCarriesOnlyItsTwoSeeds is [ER-3]'s half of the
// payload law, and it exists so the law is RE-STATED rather than quietly
// widened (C-CALV4-EDITOR-R2b, [ER-3] SIGNED).
//
// [DC-1] governs the CARD'S PER-EVENT FIELD SET and the test above enforces it
// by reflection over dayCardEvent / dayCardDay. That reflection says nothing
// about the WRAPPER — a third top-level key could arrive tomorrow and every
// assertion above would stay green. R2-2b added exactly one (`members`, the
// Owner-only audience roster), which is precisely the moment to pin the
// wrapper's own inventory: two keys, both named here, and the roster's own
// field set named beside them.
//
// A THIRD SEED IS A DECISION, NOT A CONVENIENCE. Whoever adds one has to come
// here, read this, and write down why it is editor seed data on the wrapper
// rather than event data on a row.
func TestDayCard_PayloadWrapperCarriesOnlyItsTwoSeeds(t *testing.T) {
	wantWrapper := []string{"calendars", "members"}
	if got := sortedKeys(jsonKeySet(t, dayCardPayload{})); !equalStrings(got, wantWrapper) {
		t.Errorf("dayCardPayload declares top-level JSON keys %v, want exactly %v — "+
			"[DC-1] SIGNED governs the per-event field set and [ER-3] SIGNED added the "+
			"Owner-only roster; a THIRD key is a ruling, not a convenience", got, wantWrapper)
	}
	wantMember := []string{"axis", "id", "initials", "name", "pattern", "role"}
	if got := sortedKeys(jsonKeySet(t, dayCardMember{})); !equalStrings(got, wantMember) {
		t.Errorf("dayCardMember declares JSON keys %v, want exactly %v — the audience row "+
			"draws hue + locked pattern + the ringed mark + name + role, and NOTHING "+
			"about a member the Bench does not already print", got, wantMember)
	}
	// The roster must not become a second place a member's availability, zone
	// or email lives. Those are the RSVP panel's and they are gated separately.
	for _, forbidden := range []string{"tz", "timezone", "email", "free", "availability", "answer"} {
		if jsonKeySet(t, dayCardMember{})[forbidden] {
			t.Errorf("dayCardMember carries %q — the audience list decides who may SEE an "+
				"event; it is not a second roster panel", forbidden)
		}
	}
}

// TestDayCard_TheAudienceRosterIsOwnerOnlyAndCostsNoSecondDerivation is the
// other half of [ER-3]: the gate, and the identity channel.
//
// THE GATE IS CanRestrict AND NOTHING ELSE. PUT …/events/:eid/visibility is
// RequireRole(RoleOwner), so a Scribe never renders the Restricted card and
// must not receive the names — and the field is deliberately not CanDelete
// borrowed, so this test drives the two apart: a mount with CanDelete but not
// CanRestrict gets no roster.
//
// THE IDENTITY PAIR IS THE RSVP PANEL'S OWN. Two surfaces that draw the same
// people must not draw them in two colours, so the hue/pattern come from
// benchRsvpIdentity on the same roster index and this asserts the agreement
// rather than trusting the call.
func TestDayCard_TheAudienceRosterIsOwnerOnlyAndCostsNoSecondDerivation(t *testing.T) {
	// DELIBERATELY UNLIKE THE ORACLE FIXTURE'S OWN NAMES. The signed oracle
	// month names Nissa and Bryn in its event data, so asserting "this name is
	// absent" against those strings would fail on the EVENT rows rather than on
	// the roster — a test that goes red for the wrong reason is worse than none.
	roster := []BenchRosterMember{
		{UserID: "u-kael", Name: "Quillon Ardent", Role: "Owner", IsOwner: true},
		{UserID: "u-nissa", Name: "Verrow Sedge", Role: "Scribe", IsCoDM: true},
		{UserID: "u-bryn", Name: "Ilmen Vast", Role: "Player"},
	}
	d, cal := dayCardFixture(t, BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner})
	src := dayCardSource{Block: &BenchBlock{Data: d}, Calendar: cal}

	owner := dayCardPayloadJSON(dayCardSeed{CanAuthor: true, CanRestrict: true, Roster: roster}, src)
	for _, want := range []string{`"members"`, `"Quillon Ardent"`, `"Verrow Sedge"`, `"Ilmen Vast"`, `"QA"`, `"Owner"`} {
		if !strings.Contains(owner, want) {
			t.Errorf("an Owner's payload is missing %s — the allow/deny roster cannot be "+
				"drawn from ids alone, which is the whole of [ER-3]", want)
		}
	}

	// EVERY OTHER FLOOR GETS NOTHING, and the key is ABSENT rather than empty:
	// `"members":[]` would be a promise of a roster this viewer is not entitled
	// to, and permission is absence all the way down.
	for _, tc := range []struct {
		name string
		seed dayCardSeed
	}{
		{"a plain Scribe", dayCardSeed{CanAuthor: true, Roster: roster}},
		{"a player", dayCardSeed{Roster: roster}},
		{"an Owner with no roster read", dayCardSeed{CanAuthor: true, CanRestrict: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := dayCardPayloadJSON(tc.seed, src)
			if strings.Contains(raw, `"members"`) {
				t.Error("the payload carries a `members` key below the Owner floor")
			}
			for _, name := range []string{"Quillon Ardent", "Verrow Sedge", "Ilmen Vast"} {
				if strings.Contains(raw, name) {
					t.Errorf("the payload names %q below the Owner floor — the audience "+
						"roster is Restricted's own data and Restricted is Owner-only", name)
				}
			}
		})
	}

	// THE PAIR AGREES WITH THE PANEL'S, INDEX FOR INDEX.
	members := dayCardMembers(roster, true)
	if len(members) != len(roster) {
		t.Fatalf("projected %d members from a roster of %d", len(members), len(roster))
	}
	for i, m := range members {
		hue, pattern := benchRsvpIdentity(i)
		if m.Axis != hue || m.Pattern != pattern {
			t.Errorf("member %d draws (%s, %s); the RSVP panel draws (%s, %s) for the same "+
				"person — one derivation, or the two surfaces disagree in colour",
				i, m.Axis, m.Pattern, hue, pattern)
		}
		if m.Initials != benchRsvpInitials(m.Name) {
			t.Errorf("member %d's mark is %q, not the panel's %q", i, m.Initials,
				benchRsvpInitials(m.Name))
		}
	}
}

// TestDayCard_APlayerPayloadCarriesNoGMMarkInAnyForm. The producer leaves
// AudienceMark nil for a viewer who is not entitled to it, so `gold` and
// `audience` are absent for a player BY CONSTRUCTION — this asserts the
// construction rather than trusting it, because the card is the first v4
// surface where a player opens something adjacent to authoring.
func TestDayCard_APlayerPayloadCarriesNoGMMarkInAnyForm(t *testing.T) {
	gmData, gmCal := dayCardFixture(t, BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner})
	gm := dayCardPayloadJSON(dayCardSeed{CanAuthor: true}, dayCardSource{Block: &BenchBlock{Data: gmData}, Calendar: gmCal})
	if !strings.Contains(gm, `"gold":true`) {
		t.Fatal("the GM payload carries no gold rail at all; the player assertion below " +
			"would be vacuous")
	}
	for _, name := range []string{"u-nissa", "u-bryn"} {
		pd, pcal := dayCardFixture(t, BlockViewer{UserID: name, Role: permissions.RolePlayer})
		raw := dayCardPayloadJSON(dayCardSeed{}, dayCardSource{Block: &BenchBlock{Data: pd}, Calendar: pcal})
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
	d, cal := dayCardFixture(t, BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner})
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

	card := buildDayCardCalendar(d, cal, true)
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

// --- 2b. the SECOND key namespace: intercalary days -------------------------
//
// DC-ICAL-2. The mirror guard above only ever iterated ORDINARY days, because
// the signed oracle fixture declares no is_intercalary month — so
// dayCardIntercalaryKey, dayCardIntercalaryOrd, dayCardIntercalaryCoords and
// dayCardIntercalaryLabel were 0.0% covered while daycard.go's header claimed
// the mirror was pinned "in either direction". The claim was true of one
// namespace and false of the other, which is the worse of the two failure modes:
// a comment that stops a later hand from adding the test.
//
// These fixtures exist because the intercalary path is the ONE place the card's
// payload does arithmetic. An intercalary day is not "day N of the rendered
// month" — Midwinter 1 is not Deepwinter 1 — so the editor pre-fills a month
// resolved by walking the adjacency rule, and getting that wrong writes the
// event to the wrong date silently. On any fantasy calendar with festival days
// that is most of them.

// dayCardIntercalarySpec is one intercalary month: its name and its length.
type dayCardIntercalarySpec struct {
	name string
	days int
}

// dayCardIntercalaryFixture splices intercalary months in AFTER the rendered
// month, which is the adjacency rule blockIntercalaryMonths defines, and
// projects the result exactly as dayCardFixture does — same viewer, same Bench
// layer set, same single filtered pass.
func dayCardIntercalaryFixture(t *testing.T, v BlockViewer, extra []Event, specs ...dayCardIntercalarySpec) (calblock.BlockData, *Calendar) {
	t.Helper()
	cal, events := oracleLedgerFixture()

	months := make([]Month, 0, len(cal.Months)+len(specs))
	months = append(months, cal.Months[0])
	for i, s := range specs {
		months = append(months, Month{
			ID: 900 + i, CalendarID: cal.ID, Name: s.name, Days: s.days,
			SortOrder: 1 + i, IsIntercalary: true,
		})
	}
	months = append(months, cal.Months[1:]...)
	cal.Months = months

	d := projectBlock(BlockProjectionInput{
		Calendar: cal, Events: append(blockCopyEvents(events), extra...),
		Viewer: v, MonthIndex: 0, Year: 1523,
	})
	d.Layers = benchBlockLayers(blockLayerPrefs{})
	return d, cal
}

// TestDayCard_IntercalaryKeysAgreeWithTheRenderedBlock is the other half of the
// mirror guard, and the half that was missing.
//
// It asserts what the ordinary-day test asserts, against the SAME rendered
// Block: every intercalary key and ordinal the payload mints appears on a node
// the widget rendered. A drift in either direction — here, or in the widget's
// own unexported intercalaryKey / intercalaryOrdKey — fails, and it fails
// loudly rather than as "the card never opens on a festival day".
func TestDayCard_IntercalaryKeysAgreeWithTheRenderedBlock(t *testing.T) {
	d, cal := dayCardIntercalaryFixture(t,
		BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}, nil,
		dayCardIntercalarySpec{name: "Midwinter", days: 2})

	if len(d.Month.Intercalary) != 2 {
		t.Fatalf("the fixture rendered %d intercalary days, want 2 — the guard would "+
			"pass vacuously, which is how this gap opened in the first place",
			len(d.Month.Intercalary))
	}

	body := seamRenderBlockData(t, d)
	rendered := map[string]bool{}
	for _, m := range dayCardAttrRe.FindAllStringSubmatch(body, -1) {
		rendered[m[1]] = true
	}
	renderedOrd := map[string]bool{}
	for _, m := range dayCardOrdRe.FindAllStringSubmatch(body, -1) {
		renderedOrd[m[1]] = true
	}

	card := buildDayCardCalendar(d, cal, true)
	seenKeys, seenOrds := 0, 0
	for _, day := range card.Days {
		if !strings.Contains(day.Ord, "i") {
			continue
		}
		seenKeys++
		seenOrds++
		if !rendered[day.Key] {
			t.Errorf("the payload's intercalary day key %q appears on no node of the "+
				"rendered Block — the mirrored intercalaryKey namespace has drifted and "+
				"the card would never open on a festival day", day.Key)
		}
		if !renderedOrd[day.Ord] {
			t.Errorf("the payload's intercalary ladder key %q appears on no node of the "+
				"rendered Block — the mirrored intercalaryOrdKey namespace has drifted "+
				"and the `Open in the Ledger` door would find no radio", day.Ord)
		}
	}
	if seenKeys != 2 || seenOrds != 2 {
		t.Fatalf("the payload carried %d intercalary days, want 2", seenKeys)
	}
	// The two namespaces must stay APART. `cal-harptos-1` is Deepwinter 1 and
	// `cal-harptos-i1` is Midwinter 1; a mirror that collapsed them would open
	// the card on the wrong day and look completely normal doing it.
	if rendered["cal-harptos-i1"] == rendered["cal-harptos-1"] && !rendered["cal-harptos-i1"] {
		t.Fatal("neither namespace rendered at all")
	}
	for _, k := range []string{"cal-harptos-i1", "cal-harptos-i2"} {
		if !rendered[k] {
			t.Errorf("the rendered Block carries no %q — the fixture is not exercising "+
				"the second namespace", k)
		}
	}
}

// TestDayCard_IntercalaryDaysResolveTheirOWNMonth is the correctness claim the
// key guard cannot make.
//
// An intercalary day's editor pre-fill is (year, its own 1-based month), NOT
// the rendered month's index. dayCardIntercalaryCoords derives it by zipping
// the payload's flat intercalary position against blockIntercalaryMonths, on
// the assumption that the two lists are positionally identical — an assumption
// nothing checked. Two intercalary months of unequal length is the shape that
// breaks a zip, so that is the shape this test uses.
func TestDayCard_IntercalaryDaysResolveTheirOWNMonth(t *testing.T) {
	// Deepwinter (index 0, month 1), then Midwinter (2 days, month 2), then
	// Highharvest (1 day, month 3), then the eleven ordinary months.
	d, cal := dayCardIntercalaryFixture(t,
		BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}, nil,
		dayCardIntercalarySpec{name: "Midwinter", days: 2},
		dayCardIntercalarySpec{name: "Highharvest", days: 1})

	card := buildDayCardCalendar(d, cal, true)
	type coord struct{ year, month, day int }
	got := map[string]coord{}
	for _, day := range card.Days {
		if strings.Contains(day.Ord, "i") {
			got[day.Label] = coord{day.Year, day.Month, day.Day}
		}
	}

	// The labels are the festival days' OWN names — blockIntercalary suffixes
	// the ordinal when a month runs longer than a day — never "1 Deepwinter".
	want := map[string]coord{
		"Midwinter 1": {1523, 2, 1},
		"Midwinter 2": {1523, 2, 2},
		"Highharvest": {1523, 3, 1},
	}
	if len(got) != len(want) {
		t.Fatalf("intercalary days = %+v, want %d entries", got, len(want))
	}
	for label, w := range want {
		g, ok := got[label]
		if !ok {
			t.Errorf("no intercalary day labelled %q — dayCardIntercalaryLabel must print "+
				"the festival's own name, not its ordinal in the rendered month", label)
			continue
		}
		if g != w {
			t.Errorf("%s resolved to (year %d, month %d, day %d), want (%d, %d, %d) — "+
				"pre-filling the RENDERED month's index writes the event to the wrong "+
				"date on every calendar that has festival days",
				label, g.year, g.month, g.day, w.year, w.month, w.day)
		}
	}

	// And the rendered month is NOT month 2 or 3, so a coords implementation
	// that simply returned d.Month.Index+1 would have failed above rather than
	// coincidentally agreeing.
	if d.Month.Index+1 == 2 {
		t.Fatal("the fixture's rendered month collides with Midwinter's — the assertion " +
			"above would pass for the wrong reason")
	}
}

// TestDayCard_IntercalaryCoordsRefuseRatherThanGuess pins the degraded branch.
//
// Month 0 is not a fallback, it is a refusal: an editor pre-filled with a month
// the day does not live in would write the event to the wrong date, so a spine
// the adjacency rule can no longer explain returns a value the module can
// decline to open on.
func TestDayCard_IntercalaryCoordsRefuseRatherThanGuess(t *testing.T) {
	cal, _ := oracleLedgerFixture()
	if _, m := dayCardIntercalaryCoords(nil, 0, 1523, 0); m != 0 {
		t.Errorf("a nil calendar resolved month %d, want 0 (a refusal, not a guess)", m)
	}
	// No intercalary month hangs off month 0 in the signed fixture, so any
	// position is past the end of the walk.
	if _, m := dayCardIntercalaryCoords(cal, 0, 1523, 0); m != 0 {
		t.Errorf("a calendar with no intercalary months resolved month %d, want 0", m)
	}
	// And a position past the end of a real walk refuses too.
	d, ical := dayCardIntercalaryFixture(t,
		BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}, nil,
		dayCardIntercalarySpec{name: "Midwinter", days: 2})
	if _, m := dayCardIntercalaryCoords(ical, d.Month.Index, 1523, 99); m != 0 {
		t.Errorf("position 99 of a 2-day walk resolved month %d, want 0", m)
	}
}

// TestDayCard_IntercalaryDegradedInputsMirrorTheWidgetsOwnFallbacks covers the
// two branches a rendered fixture cannot reach through the Bench, and it is
// pinned against a RENDERED Block for the same reason everything else here is.
//
// An unslugged calendar keys as `cal-iN` because the widget's intercalaryKey
// does (helpers.go:599-604); an unnamed festival day falls back to its ordinal
// because there is nothing else honest to print. Both are one `if` in a
// mirrored function, and one `if` is exactly the size of drift that ships.
func TestDayCard_IntercalaryDegradedInputsMirrorTheWidgetsOwnFallbacks(t *testing.T) {
	d, cal := dayCardIntercalaryFixture(t,
		BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}, nil,
		dayCardIntercalarySpec{name: "  ", days: 1}) // whitespace-only: no name at all
	d.CalendarSlug = ""

	body := seamRenderBlockData(t, d)
	if !strings.Contains(body, `data-day="cal-i1"`) {
		t.Fatal("the rendered Block does not key an unslugged calendar as `cal-i1`; " +
			"the mirror's fallback is being measured against the wrong thing")
	}

	card := buildDayCardCalendar(d, cal, true)
	var ical *dayCardDay
	for i := range card.Days {
		if card.Days[i].Ord == "i1" {
			ical = &card.Days[i]
			break
		}
	}
	if ical == nil {
		t.Fatal("the payload carries no intercalary day")
	}
	if ical.Key != "cal-i1" {
		t.Errorf("an unslugged calendar keyed its festival day %q, want %q — the widget's "+
			"own fallback is `cal`", ical.Key, "cal-i1")
	}
	if ical.Label != "1" {
		t.Errorf("an unnamed festival day labelled %q, want its ordinal %q — there is "+
			"nothing else honest to print", ical.Label, "1")
	}
}

// TestDayCard_IntercalaryDaysCarryTheirMarks closes the loop: the agreement law
// applies to the second namespace too.
//
// An event on an intercalary day reaches the payload through the SAME viewer
// filter as a grid event, from the same single pass, so a dm_only festival is
// absent from a player's payload for the same reason it is absent from their
// Ledger — the producer never sent it, not because the card hid it.
func TestDayCard_IntercalaryDaysCarryTheirMarks(t *testing.T) {
	vigil := Event{ID: "ev-vigil", CalendarID: "cal-harptos", Name: "Midwinter Vigil",
		Year: 1523, Month: 2, Day: 1, Visibility: "everyone"}
	rite := Event{ID: "ev-rite", CalendarID: "cal-harptos", Name: "The pale rite",
		Year: 1523, Month: 2, Day: 1, Visibility: "dm_only"}

	ids := func(v BlockViewer) []string {
		d, cal := dayCardIntercalaryFixture(t, v, []Event{vigil, rite},
			dayCardIntercalarySpec{name: "Midwinter", days: 2})
		var out []string
		for _, day := range buildDayCardCalendar(d, cal, true).Days {
			if day.Ord != "i1" {
				continue
			}
			for _, e := range day.Events {
				out = append(out, e.ID)
			}
		}
		sort.Strings(out)
		return out
	}

	gm := ids(BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner})
	if len(gm) != 2 {
		t.Fatalf("the GM's Midwinter 1 carries %v, want both events", gm)
	}
	player := ids(BlockViewer{UserID: "u-bryn", Role: permissions.RolePlayer})
	if len(player) != 1 || player[0] != "ev-vigil" {
		t.Fatalf("the player's Midwinter 1 carries %v, want just the public vigil — "+
			"permission is ABSENCE and the producer is where it happens", player)
	}
}

// TestDayCard_LedgerDockedMirrorsTheWidgetsOwnPredicate. Absence has TWO
// causes and the card's Ledger door depends on telling them apart, so both are
// exercised: a host that never docked the zone (no `ledger` layer) and a viewer
// whose host docked it hidden (LedgerHidden).
func TestDayCard_LedgerDockedMirrorsTheWidgetsOwnPredicate(t *testing.T) {
	base, baseCal := dayCardFixture(t, BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner})
	if !dayCardLedgerDocked(base) {
		t.Fatal("the Bench's own layer set should dock the Ledger")
	}

	off := base
	off.Layers = calblock.LayerState{Enabled: []string{"moons", "eras", "weeknums", "shelf"}}
	if dayCardLedgerDocked(off) {
		t.Error("a viewer who switched the `ledger` layer off has no docked Ledger")
	}
	if buildDayCardCalendar(off, baseCal, true).LedgerDocked {
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
	if len(buildDayCardCalendar(off, baseCal, true).Days) == 0 {
		t.Error("with the Ledger switched off the card is the ONLY answer, and it must " +
			"still carry every day")
	}
}

// TestDayCard_WeekdayComesFromTheCalendarAndNotFromASeven. The fixture's month
// is a TEN-day week. A `% 7` anywhere in the derivation would name the wrong
// weekday on every calendar in the product that is not Gregorian, and the
// failure reads as a cosmetic typo rather than as a broken rule.
func TestDayCard_WeekdayComesFromTheCalendarAndNotFromASeven(t *testing.T) {
	d, cal := dayCardFixture(t, BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner})
	if d.Month.WeekLen != 10 {
		t.Fatalf("the fixture's week is %d days; this assertion needs the ten-day one",
			d.Month.WeekLen)
	}
	card := buildDayCardCalendar(d, cal, true)
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
// is asserted separately below. A Bench with no Block gets NO scaffold and no
// stylesheet — orphan DOM keyed to invokers that do not exist is the same thing
// bench.templ's header refuses for popovers().
//
// THE DRIVER IS NO LONGER PART OF THIS ASSERTION and its absence here is the
// hotfix, not an erosion. calendar_daycard.js used to be a page-side <script>
// mounted beside this scaffold; because the scaffold lives inside
// <main id="main-content">, htmx DELETED that tag on every boosted sidebar
// navigation (allowScriptTags=false), so the card wired on a direct load and
// silently did not when reached through the sidebar. The driver now ships from
// the plugin body-script registry, which base.templ emits outside the swapped
// region — pinned by TestPermissionsDriverIsNeverMountedWithoutItsDependency
// and TestBenchPageMountsNoPageSideScript. The SCAFFOLD's presence-per-role and
// absence-without-a-payload are what this test still owns, and both are
// unchanged.
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
	} {
		if strings.Contains(body, bad) {
			t.Errorf("a Bench with no Block still emits %q — there is no day to open on", bad)
		}
	}
}

// TestBenchPageMountsNoScriptInsideTheSwappedRegion is the durable form of the
// hotfix.
//
// THE DEFECT IN ONE SENTENCE: a <script src> anywhere in the Bench page body is
// inside <main id="main-content">, because that is where the App layout puts
// {children...}; every sidebar link is hx-boost="true"
// hx-select="#main-content" hx-swap="innerHTML"; and with
// htmx.config.allowScriptTags false (boot.js) htmx's makeFragment does not skip
// those tags, it REMOVES them. So a page-side script is present on a direct load
// and absent on a boosted one, and nothing in the rendered page looks different,
// because the stylesheets in the same region survive.
//
// THE SCOPE IS THE SWAPPED REGION, not the document. The shell's own ~59 script
// tags sit outside <main> and are exactly what a boosted navigation is designed
// to keep; counting those would be measuring the layout. What must be zero is
// scripts INSIDE #main-content, for every role and with and without a day-card
// payload. Anything the Bench needs goes through the plugin body-script registry
// (internal/app/routes.go → layouts.SetPluginBodyScripts → base.templ), which
// emits after {children...}.
//
// This test renders with an empty context, so the registry contributes nothing
// here by construction — which is exactly why the registry's own contents are
// pinned separately, in TestBenchDriversShipFromThePluginBodyScriptRegistry.
func TestBenchPageMountsNoScriptInsideTheSwappedRegion(t *testing.T) {
	bare := benchFxData(true, true)
	bare.Primary, bare.RealWorld, bare.DayCardJSON = nil, nil, ""
	empty := benchFxData(true, true)
	empty.CalendarCount = 0
	for name, data := range map[string]BenchData{
		"owner":       benchFxData(true, true),
		"player":      benchFxData(false, false),
		"no-block":    bare,
		"owner-empty": empty,
	} {
		swapped := benchSwappedRegion(t, renderBench(t, data))
		if n := strings.Count(swapped, "<script"); n != 0 {
			t.Errorf("%s: the Bench page emits %d <script> tag(s) inside #main-content; htmx DELETES "+
				"every one of them on a boosted sidebar navigation (allowScriptTags=false), so the "+
				"surface wires on a direct load and silently does not through the sidebar. Contribute "+
				"it to the plugin body-script registry in internal/app/routes.go instead.", name, n)
		}
	}
}

// benchSwappedRegion returns the substring htmx would keep on a boosted sidebar
// navigation: the contents of <main id="main-content">. Both bounds are checked
// before they are used, so a layout rename fails loudly here instead of silently
// reducing every caller to an assertion over the empty string (COMMON §3).
func benchSwappedRegion(t *testing.T, page string) string {
	t.Helper()
	const marker = `id="main-content"`
	at := strings.Index(page, marker)
	if at < 0 {
		t.Fatalf("no %s in the rendered page — the App layout's swap target was renamed and every "+
			"boosted-navigation assertion in this file just stopped reading anything", marker)
	}
	open := strings.Index(page[at:], ">")
	if open < 0 {
		t.Fatal("unterminated <main> open tag in the rendered page")
	}
	rest := page[at+open+1:]
	end := strings.Index(rest, "</main>")
	if end < 0 {
		t.Fatal("no </main> in the rendered page")
	}
	return rest[:end]
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

	// A PLAYER'S CARD IS READ-ONLY AND COMPLETE. It lists the day's events —
	// the same set the Ledger would — with no `+ New event`, no row-click into
	// an editor, no disabled controls, no `title` explaining an absence and no
	// count of anything hidden. The editor's whole scaffold is gone, not
	// greyed: every field name, every marker, every route string.
	for _, bad := range []string{
		"data-dc-new", "data-dc-can-edit", "data-cal-dayeditor", "New event",
		"data-de-form", "data-de-name", "data-de-gmonly", "data-de-delete",
		"data-de-category", "needs backend</span>",
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

// TestDayCard_TheEditorIsRenderedExactlyAtItsFloors is [DC-9]'s markup-level
// enforcement, and it is asserted THREE WAYS because the three gates are three
// different axes:
//
//	the card and its rows      Player   — reading a day is a Player affordance
//	+ New event, edit, Save    Scribe   — the shipped create/update floor
//	dm_only                    CanAuthorDmOnly() — the CAPABILITY, not the role
//	Delete                     Owner    — the shipped DELETE floor
//
// Reading the mockup's owner / co-DM / player as one ladder would ship a Scribe
// the co-DM's powers, which is the exact failure the ruling was signed to stop.
func TestDayCard_TheEditorIsRenderedExactlyAtItsFloors(t *testing.T) {
	base := benchFxData(true, true)

	for _, tc := range []struct {
		name    string
		mount   DayCardMount
		present []string
		absent  []string
	}{
		{
			name:    "a plain Scribe authors, but not as a co-DM and not as an Owner",
			mount:   DayCardMount{CanCreate: true, CampaignID: "camp-1"},
			present: []string{"data-dc-can-edit", "data-dc-new", "data-cal-dayeditor", "data-de-save"},
			// THE WHOLE VISIBILITY FIELDSET IS GONE, not collapsed and not a
			// radio group of one: a Scribe with neither capability has exactly
			// one lawful visibility, so there is no choice to render and
			// nothing narrates its absence. editor-scribe-light.png is that
			// state.
			absent: []string{
				"data-de-gmonly", "data-de-delete", "data-de-restricted",
				"data-de-aud", "data-de-visibility",
			},
		},
		{
			name:    "a co-DM Scribe gains dm_only and nothing else",
			mount:   DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CampaignID: "camp-1"},
			present: []string{"data-de-gmonly"},
			absent:  []string{"data-de-delete"},
		},
		{
			// EXTENDED for CanRestrict (C-CALV4-EDITOR-R2b stage 2, [ER-3]
			// SIGNED). A co-DM has the dm_only CAPABILITY and neither Owner
			// floor, so the ◈ Restricted card and its audience roster are
			// absent from their DOM while ◥ DM only is present — which is the
			// shortest proof the two axes did not collapse into one.
			name:    "a co-DM gains dm_only and NEITHER Owner gate",
			mount:   DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CampaignID: "camp-1"},
			present: []string{"data-de-gmonly", "data-de-visibility"},
			absent:  []string{"data-de-delete", "data-de-restricted", "data-de-aud"},
		},
		{
			name:    "an Owner gains Delete AND Restricted",
			mount:   DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CanDelete: true, CanRestrict: true, CampaignID: "camp-1"},
			present: []string{"data-de-gmonly", "data-de-delete", "data-de-restricted", "data-de-aud"},
		},
		{
			// AN OWNER WITHOUT THE dm_only CAPABILITY CANNOT EXIST — Step 0
			// confirmed CanAuthorDmOnly() is `MemberRole >= RoleOwner ||
			// IsDmGranted` (campaigns/model.go:277), so an Owner always has it
			// and this table must never construct the fourth combination. What
			// IS constructible and worth pinning is the reverse: the Restricted
			// card without CanDelete, which proves the two Owner floors are two
			// fields rather than one borrowed twice.
			name:    "Restricted is its own gate, not Delete's",
			mount:   DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CanRestrict: true, CampaignID: "camp-1"},
			present: []string{"data-de-restricted", "data-de-aud"},
			absent:  []string{"data-de-delete"},
		},
		{
			name:   "a player receives the card and no editor at all",
			mount:  DayCardMount{CampaignID: "camp-1"},
			absent: []string{"data-dc-can-edit", "data-dc-new", "data-cal-dayeditor", "data-de-save"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := base
			data.DayCard = tc.mount
			body := renderBench(t, data)
			// The card itself mounts at every floor — that is the Player half.
			if !strings.Contains(body, "data-cal-daycard") {
				t.Fatal("the card did not mount; every assertion below would be vacuous")
			}
			for _, want := range tc.present {
				if !strings.Contains(body, want) {
					t.Errorf("missing %q at this floor", want)
				}
			}
			for _, bad := range tc.absent {
				if strings.Contains(body, bad) {
					t.Errorf("%q is rendered above this viewer's floor — permission is "+
						"ABSENCE, not a disabled control", bad)
				}
			}
		})
	}
}

// TestDayCard_TheEditorShipsNoRationaleAndNoUnbackedChip.
//
// EXTENDED BY NAME, C-CALV4-EDITOR-R2b stage 2, per §8 of the dispatch and
// under [ER-10] SIGNED.
//
// WHO TURNED IT OVER AND WHAT THE OLD CLAIM WAS. Stage 2 shipped no chips at
// all, so this test could COUNT ZERO: the literal "needs backend" simply did
// not appear in the editor. The chrome adds the LEGITIMATE chips — the units
// `recurrence_type` does not accept, and the day-of-week multi-select whose
// column exists but whose expansion is base-anchored and ignores it — so a
// count of zero would now be a test forcing the honest disclosure OUT of the
// product.
//
// WHY THE NEW CLAIM IS NOT WEAKER, which is [ER-10]'s bar. The old claim was
// "no chips"; the new one is STRICTLY MORE SPECIFIC: every `.badge.need` in
// the editor carries the literal words "needs backend" and nothing else, the
// number of them is pinned, and each one is required to sit beside a control
// this file NAMES as unbacked. A chip on a shipped field — the mockup's two
// wrong ones, over in-world time and "Lasts N days" — still fails, and so does
// a chip that says anything other than "needs backend". The rationale half of
// the test (`.hint`, the audience builder, the change-owner row) is untouched.
//
// MUTATION-TESTED: chipping the month unit (a backed type) turns this red on
// the count; changing a chip's text to "coming soon" turns it red on the
// literal; adding a fourth chip turns it red on the inventory.
func TestDayCard_TheEditorShipsNoRationaleAndNoUnbackedChip(t *testing.T) {
	data := benchFxData(true, true)
	data.DayCard = DayCardMount{
		CanCreate: true, CanAuthorDmOnly: true, CanDelete: true, CanRestrict: true,
		CampaignID: "camp-1",
	}
	body := renderBench(t, data)

	i := strings.Index(body, "cal-dayeditor")
	if i < 0 {
		t.Fatal("the editor did not render")
	}
	editor := body[i:]
	if j := strings.Index(editor, "</form>"); j > 0 {
		editor = editor[:j]
	}

	// ── THE RATIONALE HALF — UNCHANGED, AND STILL ZERO ────────────────────
	for _, bad := range []string{
		`class="hint"`,
		// The audience BUILDER (tags, "resolves to 3 of 5") and the change-owner
		// row do not exist. The per-member allow/deny roster does — it is the
		// shape visibility_rules actually carries — and it is built by the
		// module from the Owner-only payload seed, not rendered here.
		"add tag or member", "resolves to", "Change owner",
		// `year` IS INVENTION: there is no yearly recurrence_type and it
		// degrades silently to one occurrence, so it does not ship at all —
		// not even chipped. An unbacked unit in a picker is a trap.
		">year<", "Umber",
		// The recurrence "ends" cycler has no write path on either shipped verb.
		"until Deepwinter ends", "no end date", "max occurrences",
	} {
		if strings.Contains(editor, bad) {
			t.Errorf("the editor renders %q — see this file's header and [DC-5] part 2", bad)
		}
	}

	// ── THE CHIP HALF — BACKED vs UNBACKED, not zero vs non-zero ──────────
	//
	// The SERVER-RENDERED editor carries exactly ONE chip: the day-of-week
	// multi-select's. The other two ride the unit list, which the module builds
	// from the calendar's own derived week — they cannot be asserted here and
	// are pinned in test/js/daycard_editor_requests.test.mjs instead, which is
	// the honest split rather than a Go test asserting about JS.
	if n := strings.Count(editor, `class="badge need"`); n != 1 {
		t.Errorf("the editor renders %d `.badge.need` chips server-side, want exactly 1 "+
			"(the day-of-week multi-select, whose column exists but whose expansion is "+
			"BASE-ANCHORED and ignores it). A chip on a shipped field is the mockup's "+
			"defect; a chip missing from an unbacked one is a silent trap", n)
	}
	// EVERY CHIP SAYS THE LITERAL WORDS. `.badge.need` means "needs backend"
	// and nothing else — a chip that said "coming soon" or "planned" would be
	// the same promise wearing a vaguer face.
	if strings.Count(editor, `class="badge need"`) != strings.Count(editor, "needs backend") {
		t.Error("a `.badge.need` in the editor does not carry the literal words " +
			"`needs backend`; the class means that phrase and nothing else")
	}

	// …and the fields whose mockup chips were WRONG are really here, UNCHIPPED.
	// The columns and the bindings exist for all of them.
	for _, want := range []string{
		"data-de-starth", "data-de-endm", "data-de-endday", "data-de-endread",
		"data-de-typerail", "data-de-datepicker", "data-de-tie",
	} {
		if !strings.Contains(editor, want) {
			t.Errorf("the editor is missing %q — the columns and the binding exist, so "+
				"the mockup's `needs backend` chip came off and the FIELD stayed", want)
		}
	}
}

// TestDayCard_TheEditorReproducesTheStillsMechanicalClaims is the other half of
// §2's instruction: "the mockup proves the drawing; your test proves the build."
//
// Every assertion here restates a number `mockups/stills/event-editor/
// _measurements.json` asserts about the DRAWING, re-taken against the PRODUCT's
// own DOM. Where a claim can only be checked in a browser (the 24px floor, the
// day-of-week wrap, the horizontal fold) it is NOT faked here — a Go test
// asserting a pixel it cannot see would be worse than no coverage, because it
// would read as coverage.
//
// ── WHERE THOSE THREE ACTUALLY GO, NAMED, BECAUSE THE HANDOFF USED TO BE
//    ADDRESSED TO NOBODY ──────────────────────────────────────────────────
//
// This header used to say they "belong to the screenshot gate", and the
// screenshot gate did not measure any of them — it carried two CAPTION STRINGS,
// "the fold holds" and "no horizontal scroll", which are prose. So §10 item 9
// rested on `min-block-size: 24px` DECLARATIONS in the sheet and §10 item 11 was
// asserted by no test and no image, while this sentence read like coverage.
// That was finding 3 of the fix round, and it is closed by naming the receiver:
//
//	· the 24px control floor  → daycard_floors_probe_test.go, DAYCARD_FLOORS=1,
//	  measured with getBoundingClientRect over every visible control on the card
//	  and editor surface AND over every ✕/✓ mark's nearest control ancestor, in
//	  CREATE and EDIT mode (the tie pill's ✕ and the allow/deny pair exist only
//	  in edit mode), at 390 / 820 / 1440, both themes.
//	· the horizontal fold     → the same file: scrollWidth === clientWidth plus
//	  zero nodes of the card/editor subtree crossing the viewport edge, same
//	  matrix. The 390 arm runs in a real 390px viewport — a nested browsing
//	  context — because `--window-size` clamps this Chromium to 500px, which is
//	  why the rejected evidence's three `-390x844` files were 500px wide.
//	· the day-of-week wrap    → still unperformed here and unperformed there,
//	  and it is BOOKED as such in the report rather than described as covered.
func TestDayCard_TheEditorReproducesTheStillsMechanicalClaims(t *testing.T) {
	data := benchFxData(true, true)
	data.DayCard = DayCardMount{
		CanCreate: true, CanAuthorDmOnly: true, CanDelete: true, CanRestrict: true,
		CampaignID: "camp-1",
	}
	body := renderBench(t, data)

	// SCOPED TO THE CARD AND THE EDITOR, DELIBERATELY, and the scope is a
	// finding rather than a convenience. The Bench at large carries BOTH of the
	// next two strings legitimately and they are signed where they sit: the
	// surface caption explains what `needs backend` means ("Tiles marked <b>needs
	// backend</b> name a gap…"), and the RSVP tile's inert Propose control is a
	// real `disabled` button with a real reason on it (C-CALV4-RSVP-P8B). This
	// slice inherits both unchanged; what it asserts is that ITS OWN surface
	// adds neither, which is the claim the stills make about the editor.
	surface := dayCardSurfaceOf(t, body)

	// `.hint` nodes = 0 — no design rationale ships as product copy.
	if n := strings.Count(surface, `class="hint"`); n != 0 {
		t.Errorf("%d `.hint` nodes on the card+editor surface; design rationale "+
			"addressed to a reviewer does not render inside the product", n)
	}
	// disabled / aria-disabled controls = 0 — PERMISSION IS ABSENCE, never
	// greying, and never a `title` explaining what is missing.
	for _, bad := range []string{"disabled", "aria-disabled"} {
		if strings.Contains(surface, bad) {
			t.Errorf("the card+editor surface renders a %q attribute — absence is "+
				"absence ([DC-9] SIGNED: markup-level, never CSS, never JS, never a "+
				"greyed control)", bad)
		}
	}
	// literal week lengths in CSS selectors = 0. Ten-day weeks are native; a
	// `nth-child(7n)` or a `repeat(7,…)` here is the exact defect
	// css_contract_test.go forbids one layer up.
	sheet := benchCommentRe.ReplaceAllString(dayCardCSS(t), " ")
	for _, bad := range []string{
		"nth-child(7", "nth-child(10", "repeat(7", "repeat(10", "repeat(5", "repeat(13",
	} {
		if strings.Contains(sheet, bad) {
			t.Errorf("calendar-daycard.css contains %q — week length is a VARIABLE, "+
				"written by the module from the calendar's own weekday names", bad)
		}
	}
	// …and the variable it uses instead is really there, so the assertion above
	// is proving a mechanism rather than an absence.
	if !strings.Contains(sheet, "var(--week-len") {
		t.Error("the date grid does not read --week-len; the assertion above would " +
			"pass on a sheet that had simply stopped drawing a grid")
	}
	// The module must not carry one either — a `% 7` in the derivation is the
	// same defect one layer in.
	mod := readRepoFile(t, "internal/plugins/calendar/static/js/calendar_daycard.js")
	code := dayCardJSLineComment.ReplaceAllString(mod, "")
	for _, bad := range []string{"% 7", "%7", "'tenday'", `"tenday"`, "* 7"} {
		if strings.Contains(code, bad) {
			t.Errorf("calendar_daycard.js contains %q — the week is DERIVED from the "+
				"payload's own weekday names (weekShape), never assumed and never named", bad)
		}
	}

	// A PLAYER MEETS NONE OF IT, and the chip count is where the chrome bites.
	// The player's SURFACE is the card alone — the editor is not rendered at
	// all below the Scribe floor, which is the gate expressed as absence.
	player := dayCardSurfaceOf(t, renderBench(t, benchFxData(false, false)))
	for _, bad := range []string{
		"needs backend", "badge need", "data-de-typerail", "data-de-datepicker",
		"data-de-visibility", "data-de-aud", "data-de-tie", "data-de-restricted",
		"data-de-recurrence", "Create event", "Save changes", "Delete event",
	} {
		if strings.Contains(player, bad) {
			t.Errorf("a player's Bench contains %q — the chrome is where a chip gets "+
				"tempting, and a chip is a promise addressed to someone who can act on it", bad)
		}
	}
}

// dayCardSurfaceOf narrows a rendered Bench to THIS SLICE'S OWN DOM: the card
// scaffold, and the editor when the viewer's floor rendered one.
//
// It exists because the assertions above are about what R2b ADDS. The Bench it
// mounts onto legitimately carries both `needs backend` (in the surface caption
// that explains the chip) and a `disabled` control (the RSVP tile's inert
// Propose, signed under C-CALV4-RSVP-P8B with its reason on it), and a
// page-wide assertion would either fail on inherited, signed DOM or have to be
// weakened until it proved nothing.
func dayCardSurfaceOf(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, `id="cal-daycard"`)
	if i < 0 {
		t.Fatal("the card did not mount; every assertion downstream would be vacuous")
	}
	return body[i:]
}

// --- 4. the sheets ---// --- 4. the sheets ----------------------------------------------------------

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
// It reads the sheet BY BRACE, not by line (DC2-SCOPEGUARD-LINEFORM, round-2
// fix-forward). The line-form version only inspected lines ENDING in `{`, so
// every rule written entirely on one line — `.x { color: red }` — was never
// examined at all, and this sheet has 20-odd of them. All were correctly scoped,
// which is precisely why the hole would have stayed invisible until it wasn't.
func TestDayCardCSS_EverySelectorIsScoped(t *testing.T) {
	sels := cssSelectors(benchCommentRe.ReplaceAllString(dayCardCSS(t), " "))
	if len(sels) < 20 {
		t.Fatalf("only %d selectors found; the parser stopped reading the sheet", len(sels))
	}
	for _, sel := range sels {
		// TWO ROOTS, BOTH THIS SHEET'S OWN, and every amendment is named rather
		// than silent: stage 2 of DAYCARD added .cal-dayeditor because the card
		// CLOSES as the editor OPENS ([DC-7]) and one box cannot be in two
		// places.
		//
		// A THIRD ROOT, AMENDED BY NAME — C-CALV4-EDITOR-R2b stage 4, under
		// [ER-8] SIGNED.
		//
		// WHO TURNED IT OVER AND WHAT THE OLD CLAIM WAS: "every selector sits
		// under .cal-daycard or .cal-dayeditor; a third root is a surface that
		// escaped its sheet." THE NEW CLAIM adds exactly one root,
		// `.cal-daycard-drag`, and it is not an escape — it is a CONSEQUENCE of
		// a signed refusal. [ER-8] forbids drag-create from marking the Block's
		// own cells, so the span preview has to be a PAGE-LEVEL overlay; and it
		// cannot be a DESCENDANT of .cal-daycard because the card is a popover
		// that is closed for the whole of the drag. The module that creates it
		// is the card's, it lives beside the card, and it exists only while a
		// drag is in flight.
		//
		// WHY THE NEW CLAIM IS NOT WEAKER: the admitted roots are LITERAL CLASS
		// NAMES, not prefixes. The check below extracts every class token in the
		// selector and requires one of them to BE one of the three names — so
		// `.cal-daycard-bogus-fourth-root` fails even though it contains
		// `.cal-daycard` as a substring, and so does any selector that merely
		// happens to mention the card inside a longer name.
		//
		// FIXED FORWARD AT STAGE 7, AND THE PREVIOUS CUT IS WORTH RECORDING.
		// This comment was written above a `strings.Contains(sel,
		// ".cal-daycard")` test, which is a PREFIX check wearing a literal's
		// clothes: `.cal-daycard-drag` passed it by accident rather than by
		// admission, and so would `.cal-daycard-anything-at-all`. A guard whose
		// comment claims a tightening it does not perform is worse than no guard
		// — it spends the reader's trust and returns nothing.
		//
		// MUTATION-TESTED (stage 7): appending
		// `.cal-daycard-bogus-fourth-root { color: red; }` to
		// static/css/calendar-daycard.css turns this RED; it was GREEN against
		// the substring cut. Renaming the overlay's class to `.dragpreview` is
		// also red. Both restored.
		if !dayCardSelectorIsRooted(sel) {
			t.Errorf("unscoped selector in calendar-daycard.css: %q — every selector in this "+
				"sheet must carry one of the three admitted roots %v as a WHOLE class name; a "+
				"fourth root is a surface that escaped its sheet",
				sel, dayCardAdmittedRoots)
		}
	}
}

// dayCardAdmittedRoots are the only class names calendar-daycard.css may own.
//
// Three, each admitted by a named ruling: `.cal-daycard` and `.cal-dayeditor`
// are DAYCARD's two boxes ([DC-7] — the card CLOSES as the editor OPENS, so one
// box cannot be in two places), and `.cal-daycard-drag` is the drag-create span
// preview, which [ER-8] SIGNED forces OUT of the Block and out of the card's
// own subtree (the card is closed for the whole of the drag).
var dayCardAdmittedRoots = []string{".cal-daycard", ".cal-dayeditor", ".cal-daycard-drag"}

// dayCardSelectorClassRe pulls every CLASS TOKEN out of a selector. The trailing
// character class is CSS's own identifier set, so `.cal-daycard-drag` reads as
// one token rather than as `.cal-daycard` plus noise — which is the entire
// difference between this check and the substring test it replaced.
var dayCardSelectorClassRe = regexp.MustCompile(`\.[A-Za-z_-][A-Za-z0-9_-]*`)

// dayCardSelectorIsRooted reports whether a selector names one of the admitted
// roots as a whole class name.
func dayCardSelectorIsRooted(sel string) bool {
	for _, tok := range dayCardSelectorClassRe.FindAllString(sel, -1) {
		for _, root := range dayCardAdmittedRoots {
			if tok == root {
				return true
			}
		}
	}
	return false
}

// cssSelectors returns every RULE selector in a comment-stripped stylesheet,
// whatever line form it was written in.
//
// A selector is the text between the previous delimiter (`{`, `}` or `;`) and
// the `{` that opens its block, which is true of a one-liner, a multi-line
// selector list and a rule nested inside an at-rule alike; a declaration can
// never precede a `{`, so declarations cannot be mistaken for selectors.
// At-rule preludes (`@media …`) are skipped — they are not selectors — but the
// rules INSIDE them are returned, which is the point: a responsive branch is
// exactly where an unscoped rule likes to hide.
func cssSelectors(code string) []string {
	var out []string
	start := 0
	for i := 0; i < len(code); i++ {
		switch code[i] {
		case '{':
			sel := strings.Join(strings.Fields(code[start:i]), " ")
			if sel != "" && !strings.HasPrefix(sel, "@") {
				out = append(out, sel)
			}
			start = i + 1
		case '}', ';':
			start = i + 1
		}
	}
	return out
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
		".cal-daycard .dc-row .dc-edit",
		// ── THE EDITOR ────────────────────────────────────────────────
		//
		// AMENDED BY NAME, C-CALV4-EDITOR-R2b stage 2, under [ER-10] SIGNED.
		//
		// WHO TURNED IT OVER AND WHAT THE OLD CLAIM WAS. Stage 2 of DAYCARD
		// shipped the editor as a MECHANISM and this list named its five
		// classes: `.de-row`, `.de-fields`, `.de-f .dc-door`, `.de-f
		// .de-danger` and `.de-form`. The chrome pass REPLACES those controls —
		// `.de-row` became `.fld`, the `.dc-door` foot buttons became `.btn`,
		// `.de-danger` became `.btn.danger` — so four of the five names now
		// describe DOM that no longer exists.
		//
		// WHY THE NEW CLAIM IS NOT WEAKER, which is [ER-10]'s real bar. The old
		// list had ELEVEN editor entries; this one has FORTY-ONE, and it covers
		// every class the module builds at runtime (the type rail's `.topt`, the
		// date grid's `.dp-c` / `.dp-ic`, the audience row's `.inimark` and its
		// allow/deny pair, the tie pill's `.rmx`, the preview's `.lrow` and its
		// gold rail) — which is precisely the DOM a stale sheet would render as
		// unstyled text while every assertion stayed green. The #568 gap this
		// test closes is WIDER after the amendment, not narrower.
		//
		// The three conditions are met in full: every `data-de-*` handle
		// survives the controls' replacement (the templ's header enumerates
		// them), the request bodies are byte-equivalent for equivalent input
		// (daycard_editor_requests.test.mjs keeps its cases and gains more), and
		// this comment is the naming. Mutation-tested: deleting the `.topt .g`
		// rule turns this red.
		".cal-dayeditor", ".cal-dayeditor[data-dc-shown]", ".cal-dayeditor.dcsheet",
		".cal-dayeditor .dcbox", ".cal-dayeditor .de-form", ".cal-dayeditor .de-lab",
		".cal-dayeditor .de-err", ".cal-dayeditor .de-f",
		// the head's three channels
		".cal-dayeditor .ed-bar", ".cal-dayeditor .ed-head", ".cal-dayeditor .tymark",
		".cal-dayeditor .tymark .rail", ".cal-dayeditor .tymark .g",
		".cal-dayeditor .dc-h", ".cal-dayeditor .readout", ".cal-dayeditor .readout.mono",
		// the shared control vocabulary
		".cal-dayeditor .btn", ".cal-dayeditor .btn.fill", ".cal-dayeditor .btn.danger",
		".cal-dayeditor .ed-head .btn", ".cal-dayeditor .in", ".cal-dayeditor .in.num",
		".cal-dayeditor .in.prose", ".cal-dayeditor .in.sel-like",
		".cal-dayeditor .in.sel-like .lb", ".cal-dayeditor .in.sel-like .ar", ".cal-dayeditor .vh",
		".cal-dayeditor .fld", ".cal-dayeditor .frow", ".cal-dayeditor .cap",
		".cal-dayeditor .badge", ".cal-dayeditor .badge.need", ".cal-dayeditor .badge.gm",
		".cal-dayeditor .gold",
		// the two-column body
		".cal-dayeditor .ed-body", ".cal-dayeditor .ed-form-col", ".cal-dayeditor .ed-side",
		// the type rail's locked triple
		".cal-dayeditor .types", ".cal-dayeditor .topt", ".cal-dayeditor .topt .rail",
		".cal-dayeditor .topt .rail.p1", ".cal-dayeditor .topt .rail.p8",
		".cal-dayeditor .topt .g", ".cal-dayeditor .topt .nm",
		// the date picker
		".cal-dayeditor .dp", ".cal-dayeditor .dp-head", ".cal-dayeditor .dp-head .hd",
		".cal-dayeditor .dp-grid", ".cal-dayeditor .dp-c", ".cal-dayeditor .dp-c .mk",
		".cal-dayeditor .dp-ic",
		// recurrence
		".cal-dayeditor .seg", ".cal-dayeditor .rec-on", ".cal-dayeditor .units",
		".cal-dayeditor .recbox", ".cal-dayeditor .wdpick",
		// visibility and its audience
		".cal-dayeditor .vis", ".cal-dayeditor .viscard", ".cal-dayeditor .viscard .nm",
		".cal-dayeditor .viscard .sub", ".cal-dayeditor .viscard .audmark",
		".cal-dayeditor .audb", ".cal-dayeditor .audrows", ".cal-dayeditor .mrow",
		".cal-dayeditor .mrow .swatch", ".cal-dayeditor .mrow .swatch.p8",
		".cal-dayeditor .mrow .inimark", ".cal-dayeditor .mrow .nm",
		".cal-dayeditor .mrow .mt", ".cal-dayeditor .mrow .ad",
		// the tie field
		".cal-dayeditor .pill", ".cal-dayeditor .pill .rmx", ".cal-dayeditor .tieres",
		// the live preview
		".cal-dayeditor .pv", ".cal-dayeditor .pv-cell", ".cal-dayeditor .pv .chip",
		".cal-dayeditor .pv .chip .rail", ".cal-dayeditor .pv .chip .tok",
		".cal-dayeditor .pv .chip .lb", ".cal-dayeditor .pv .dogear",
		".cal-dayeditor .pv .audmark", ".cal-dayeditor .pv .lrow",
		".cal-dayeditor .pv .lrow .dg", ".cal-dayeditor .pv .lrow .rail",
		".cal-dayeditor .pv .lrow .gr", ".cal-dayeditor .pv .lrow .tok",
		".cal-dayeditor .pv .lrow .mid", ".cal-dayeditor .pv .lrow .nm",
		".cal-dayeditor .pv .lrow .audchip",
		// drag-create's page-level span preview (stage 4, [ER-8] SIGNED)
		".cal-daycard-drag", ".cal-daycard-drag .dragbox",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("calendar-daycard.css does not define %q", want)
		}
	}
}

// TestDayCardCSS_NeverStylesTheOcclusionReport pins the other half of
// DC-CLEAR-1's fix.
//
// `data-dc-clear` is a REPORT, not a state. [DC-3] SIGNED makes an occluded
// Ledger a STOP-AND-FLAG — a build-time refusal — and the module writes the
// attribute so §12's screenshot gate can read a rendered fact instead of
// re-deriving two rects. The moment a sheet styles it, the condition acquires a
// look, a look reads as a designed state, and a designed state is a thing
// somebody decided to ship rather than a thing somebody must fix. Nobody signed
// that. The attribute is measured and never painted.
func TestDayCardCSS_NeverStylesTheOcclusionReport(t *testing.T) {
	for _, name := range []string{"calendar-daycard.css", "calendar-bench.css"} {
		body := readRepoFile(t, filepath.Join("static", "css", name))
		code := benchCommentRe.ReplaceAllString(body, " ")
		if strings.Contains(code, "data-dc-clear") {
			t.Errorf("%s selects on data-dc-clear — the occlusion report is a "+
				"measurement for the §12 gate, not a UI state anybody designed "+
				"([DC-3] makes it a STOP-AND-FLAG, which is a thing to fix and not "+
				"a thing to paint)", name)
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
		// DC-CLEAR-1: placeCard's `clear` has a READER. The first cut computed
		// it, promised in a comment that an unclearable geometry would be
		// "visible rather than silent", and then dropped the value — so the one
		// condition [DC-3] signed as a STOP-AND-FLAG shipped as the quietest
		// thing on the page. Both names below are the reader.
		"occlusionReporter",
		"applyPlacement",
		"data-dc-clear",
		// ── EXTENDED, C-CALV4-EDITOR-R2b stage 3 ([ER-7] SIGNED) ──────
		//
		// THE CARVE-OUT'S CLASS AND ITS REDUCED-MOTION BRANCH. The morph is the
		// operator's signed ask and the reason this slice exists in the shape it
		// does, so its two load-bearing names are pinned the way the register's
		// own token already was: `edmorph` is the one named signature, and
		// `edMorphSeed` is where reduced motion DECLINES TO SEED — which is the
		// half of "instant AND complete" that lives in JS rather than in CSS.
		//
		// The failure this catches is specific and it is the one the signature's
		// second word names: a box that arrives instantly at a MID-MORPH
		// geometry because the JS wrote a start state and the CSS had no
		// transition to leave it. Deleting the reduced-motion check turns
		// test/js/daycard_open_close.test.mjs red on the END state; deleting the
		// class turns TestBenchCSS_TheEditorMorphIsTheOnlyCarveOut red on its
		// two-file claim. This is the third leg, at the source level, where the
		// other two cannot see.
		"edmorph",
		"edMorphSeed",
		"edMorphSettle",
		"--disc-open", // the settle is scheduled off the sheet's own token
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
		// A LOCAL COPY OF THE VISIBILITY MAPPING would be its third
		// ([DC-10] SIGNED refuses one), and it is the copy nobody notices
		// going stale.
		"visibility_rules: json", "allowed_users",
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

// jsonKeySet is the payload types' inventory, taken from the TYPE rather than
// from any value of it (DC2-PAYLOAD-OMITEMPTY).
//
// The whole point is that it cannot be fooled by `omitempty`: marshalling a
// literal enumerates only the fields whoever wrote the literal remembered to
// populate, so a new optional field is invisible to it and the payload's own
// law goes unenforced. A field tagged `json:"-"` is deliberately EXCLUDED —
// it never reaches the wire, and the law is about the wire.
func jsonKeySet(t *testing.T, v any) map[string]bool {
	t.Helper()
	typ := reflect.TypeOf(v)
	if typ == nil || typ.Kind() != reflect.Struct {
		t.Fatalf("jsonKeySet wants a struct, got %v", typ)
	}
	out := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.PkgPath != "" {
			continue // unexported: never marshalled
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = true
	}
	return out
}

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

// TestDayCardCSS_TheEditorNeverRepaintsTheBenchsPrimaryAction is NEW with
// C-CALV4-EDITOR-R2b stage 8, and it exists because the defect it pins shipped
// in fifteen consecutive screenshots without anyone seeing it — including the
// person who took them.
//
// WHAT WENT WRONG. `.cal-bench .btn.fill` (calendar-bench.css) paints
// `background: var(--accent)` with near-white ink. `.cal-dayeditor .btn.fill`
// (calendar-daycard.css) painted `color: var(--accent)`. Both selectors are
// specificity (0,2,0) and bench.templ links the day card's sheet SECOND, so the
// later `color` won on cascade order alone and the editor's `Create event` /
// `Save changes` rendered as the accent ON the accent — 1.0:1, measured at
// 10,933px of rgb(79,107,239) with the glyph strokes at rgb(78,106,239). The
// label was not hard to read; it was not there.
//
// WHY A GUARD AND NOT JUST A FIX. Cascade-order collisions between two sheets
// that style the same class are invisible in both files: each one reads
// correctly on its own, and the bug lives in the `<link>` order in a third.
// This test reads BOTH sheets and refuses the overlap directly.
//
// THE RULE: where calendar-daycard.css styles a class that calendar-bench.css
// gives a `background`, the day card's sheet may not set `color` to the SAME
// value without also setting a background of its own. Same token for ink and
// fill is 1.0:1 by definition, and no measurement is needed to know it.
//
// MUTATION-TESTED (stage 8): restoring `color: var(--accent)` to
// `.cal-dayeditor .btn.fill` turns this RED, naming the class, the token and
// the two files. Restored.
func TestDayCardCSS_TheEditorNeverRepaintsTheBenchsPrimaryAction(t *testing.T) {
	bench := benchCommentRe.ReplaceAllString(benchCSS(t), " ")
	card := benchCommentRe.ReplaceAllString(dayCardCSS(t), " ")

	// The Bench's fills, by the class they key off: `.btn.fill` → `var(--accent)`.
	fills := map[string]string{}
	for _, rule := range benchCSSRules(bench) {
		bg := dayCardDeclValue(rule[1], "background")
		if bg == "" {
			bg = dayCardDeclValue(rule[1], "background-color")
		}
		if bg == "" {
			continue
		}
		for _, cls := range dayCardTrailingClasses(rule[0]) {
			fills[cls] = bg
		}
	}
	if len(fills) == 0 {
		t.Fatal("calendar-bench.css declares no backgrounds at all; the parser stopped " +
			"reading and every claim below would pass vacuously")
	}
	// `.btn.fill` is the one this was caught on, so its presence is asserted
	// rather than assumed — a rename that emptied the map would otherwise make
	// this test green by disappearing.
	if _, ok := fills[".fill"]; !ok {
		t.Fatal("calendar-bench.css no longer gives `.fill` a background; the primary " +
			"action's treatment moved and this guard is pointing at nothing")
	}

	for _, rule := range benchCSSRules(card) {
		ink := dayCardDeclValue(rule[1], "color")
		if ink == "" {
			continue
		}
		if dayCardDeclValue(rule[1], "background") != "" ||
			dayCardDeclValue(rule[1], "background-color") != "" {
			// It repaints the fill too, so ink and fill are decided together
			// and this guard has nothing to say about them.
			continue
		}
		for _, cls := range dayCardTrailingClasses(rule[0]) {
			bg, shared := fills[cls]
			if !shared || bg != ink {
				continue
			}
			t.Errorf("`%s` in calendar-daycard.css sets color:%s while calendar-bench.css "+
				"gives %q background:%s — same specificity, and the day card's sheet is "+
				"linked SECOND, so this paints the label in the fill it sits on. That is "+
				"1.0:1 by definition, and it shipped in fifteen screenshots",
				rule[0], ink, cls, bg)
		}
	}
}

// dayCardDeclValue pulls one declaration's value out of a rule body. It reads
// the LAST occurrence, because a later declaration in the same block wins.
func dayCardDeclValue(body, prop string) string {
	want := ""
	for _, decl := range strings.Split(body, ";") {
		parts := strings.SplitN(decl, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) != prop {
			continue
		}
		want = strings.TrimSpace(parts[1])
	}
	return want
}

// dayCardTrailingClasses returns the class token(s) the LAST compound of each
// comma-separated selector carries — which is the element the declaration
// actually paints. `.cal-dayeditor .btn.fill` yields `.btn` and `.fill`.
func dayCardTrailingClasses(sel string) []string {
	var out []string
	for _, one := range strings.Split(sel, ",") {
		fields := strings.Fields(strings.TrimSpace(one))
		if len(fields) == 0 {
			continue
		}
		last := fields[len(fields)-1]
		// Drop pseudo-classes and -elements: `:hover` is a state, not the
		// resting paint this guard is about.
		if i := strings.Index(last, ":"); i >= 0 {
			last = last[:i]
		}
		out = append(out, dayCardSelectorClassRe.FindAllString(last, -1)...)
	}
	return out
}

// TestDayCard_TheTypePaletteCarriesTheLockedTriple is NEW with
// C-CALV4-EDITOR-R2b stage 8.
//
// THE DEFECT IT PINS. `dayCardCategory` shipped with Slug, Name, Glyph and Axis
// and no PATTERN. The module's type rail falls back to `p1` for a category that
// carries none, so every option in the rail drew the same solid stroke: the
// greyscale identity channel existed as a class name and carried no
// information, and two types were separable BY HUE ALONE. That is the one thing
// this arc's build law forbids in terms — "every hue pairs with a pattern or a
// glyph" — and it was invisible in review because the markup looked right.
//
// WHY THE PATTERN IS DERIVED AND NOT STORED. `blockMarkFor` locks an event's
// mark to `blockPatternFor(key)` where the key is the category slug
// (`blockEventAxisKey`). Deriving the palette's pattern from the same call means
// a type wears ONE identity across the grid, the Ledger and the editor. A second
// derivation would be a second identity for the same category, which is the
// defect one layer up.
//
// MUTATION-TESTED (stage 8): emitting `Pattern: ""` from dayCardCategories
// turns this RED on the first category. Restored.
func TestDayCard_TheTypePaletteCarriesTheLockedTriple(t *testing.T) {
	cal := benchFxHarptos()
	cal.EventCategories = []EventCategory{
		{Slug: "festival", Name: "Festival", Icon: "❋", Color: "#f59e0b"},
		{Slug: "quest", Name: "Quest", Icon: "▲", Color: "#8b5cf6"},
		{Slug: "battle", Name: "Battle", Icon: "⚔", Color: "#ef4444"},
	}
	cats := dayCardCategories(&cal, true)
	if len(cats) != 3 {
		t.Fatalf("the palette lost entries: %d of 3", len(cats))
	}

	seen := map[string]string{}
	for _, c := range cats {
		if c.Pattern == "" {
			t.Errorf("category %q carries no pattern — the module falls back to p1 for it, "+
				"so its rail is separable from every other type BY HUE ALONE", c.Slug)
			continue
		}
		if !dayCardPat.MatchString(c.Pattern) {
			t.Errorf("category %q carries pattern %q, which is not in the closed p1..p8 set "+
				"the sheet implements", c.Slug, c.Pattern)
		}
		// THE IDENTITY IS THE BLOCK'S, NOT A SECOND ONE. Same key, same call,
		// so a type's rail in the editor is the stroke its events wear in the
		// grid — asserted by recomputing it here rather than by trusting it.
		if want := blockPatternFor(c.Slug); c.Pattern != want {
			t.Errorf("category %q wears %q in the type rail but %q on its marks — one "+
				"category, two identities", c.Slug, c.Pattern, want)
		}
		seen[c.Pattern] = c.Slug
	}

	// THE CHANNEL MUST SEPARATE, WHICH IS A WEAKER CLAIM THAN "ALL DISTINCT"
	// AND IS THE HONEST ONE. blockPatternFor is FNV-1a mod 8, so two slugs CAN
	// collide — `quest` and `festival` both land on p6 in the shipped default
	// set — and that collision is the Block's own, inherited by locking to it
	// rather than introduced here. Asserting all-distinct would demand this
	// slice fork the identity scheme, which is the defect it is fixing, one
	// layer up. What the defect actually was is that the palette collapsed to
	// ONE pattern for every type; that is what this refuses.
	if len(seen) < 2 {
		t.Errorf("the whole palette wears %d distinct pattern(s) — the greyscale channel "+
			"has collapsed and hue is carrying the identity alone", len(seen))
	}

	// …and the other two channels are still there, because the claim is a
	// TRIPLE and a test that checked only the new leg would let one of the old
	// ones fall out unnoticed.
	if cats[0].Axis == "" || cats[0].Glyph == "" {
		t.Errorf("the locked triple lost a channel: axis=%q glyph=%q",
			cats[0].Axis, cats[0].Glyph)
	}
}
