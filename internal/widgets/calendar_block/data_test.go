package calendar_block

import (
	"reflect"
	"testing"
)

// This file is the SHAPE PIN for the calendar-v4 wave-1 cross-slice contract
// declared in data.go. Its only job is to fail loudly if a later hand renames,
// retypes, reorders or drops a field that a producer in another package depends
// on.
//
// Two deliberate properties, both required by C-CALV4-FOUNDATION-P0:
//
//  1. It is a SHAPE pin, not a BEHAVIOUR pin. It asserts nothing about what the
//     Block renders — that belongs to C-CALV4-BLOCK-P1.
//  2. It reads NO SOURCE TEXT. Every assertion goes through reflect. The
//     repo's `os.ReadFile`-plus-substring pins (see the wave COMMON brief §3)
//     red CI on pure restructuring; reflection cannot. Reformat data.go, move a
//     comment, rewrap a doc block — this test does not care. Rename a field and
//     it fails on the exact field.

// pinnedField is one expected exported field: its declaration-order position is
// implied by its index in the slice, so an insertion in the middle fails too.
type pinnedField struct {
	name string
	kind reflect.Kind
	// typeName is the reflect type name for composite fields — the struct name
	// for a struct, the ELEMENT struct name for a slice, the pointed-to struct
	// name for a pointer. Empty means "don't check" (used for the primitives,
	// where kind already says everything).
	typeName string
}

// assertShape compares a struct type's exported fields against the pin,
// in declaration order.
func assertShape(t *testing.T, typ reflect.Type, want []pinnedField) {
	t.Helper()

	if typ.Kind() != reflect.Struct {
		t.Fatalf("%s: expected a struct type, got %s", typ.Name(), typ.Kind())
	}

	var got []pinnedField
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		got = append(got, pinnedField{
			name:     f.Name,
			kind:     f.Type.Kind(),
			typeName: elementTypeName(f.Type),
		})
	}

	if len(got) != len(want) {
		t.Errorf("%s: exported field count = %d, pin expects %d\n  got:  %v\n  want: %v",
			typ.Name(), len(got), len(want), got, want)
	}

	n := len(got)
	if len(want) < n {
		n = len(want)
	}
	for i := 0; i < n; i++ {
		if got[i].name != want[i].name {
			t.Errorf("%s field %d: name = %q, pin expects %q — a producer in another package reads this name",
				typ.Name(), i, got[i].name, want[i].name)
			continue
		}
		if got[i].kind != want[i].kind {
			t.Errorf("%s.%s: kind = %s, pin expects %s",
				typ.Name(), got[i].name, got[i].kind, want[i].kind)
		}
		if want[i].typeName != "" && got[i].typeName != want[i].typeName {
			t.Errorf("%s.%s: composite type = %q, pin expects %q",
				typ.Name(), got[i].name, got[i].typeName, want[i].typeName)
		}
	}
}

// elementTypeName unwraps slices and pointers to the underlying named type, so
// the pin can say "[]DayCell" as "DayCell" without parsing a type string.
func elementTypeName(t reflect.Type) string {
	for t.Kind() == reflect.Slice || t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

func TestBlockData_ShapePinned(t *testing.T) {
	assertShape(t, reflect.TypeOf(BlockData{}), []pinnedField{
		{name: "CalendarID", kind: reflect.String},
		{name: "CalendarSlug", kind: reflect.String},
		{name: "Name", kind: reflect.String},
		{name: "CalHue", kind: reflect.String},
		{name: "Pattern", kind: reflect.String},
		{name: "Letter", kind: reflect.String},
		{name: "IsRealWorld", kind: reflect.Bool},
		{name: "IsDefault", kind: reflect.Bool},
		{name: "IsActive", kind: reflect.Bool},
		{name: "DateLabel", kind: reflect.String},
		{name: "SeasonLabel", kind: reflect.String},
		{name: "EraLabel", kind: reflect.String},
		{name: "Fault", kind: reflect.String},
		{name: "Month", kind: reflect.Struct, typeName: "MonthGeometry"},
		{name: "Sync", kind: reflect.Struct, typeName: "SyncPill"},
		{name: "Layers", kind: reflect.Struct, typeName: "LayerState"},
		{name: "Ledger", kind: reflect.Struct, typeName: "LedgerStub"},
		{name: "Shelf", kind: reflect.Struct, typeName: "ShelfStub"},
		{name: "Viewer", kind: reflect.Struct, typeName: "ViewerContext"},
	})
}

func TestMonthGeometry_ShapePinned(t *testing.T) {
	assertShape(t, reflect.TypeOf(MonthGeometry{}), []pinnedField{
		{name: "Index", kind: reflect.Int},
		{name: "Year", kind: reflect.Int},
		{name: "Name", kind: reflect.String},
		// WeekLen is the ten-day-week contract: never defaulted to 7, in CSS or Go.
		{name: "WeekLen", kind: reflect.Int},
		{name: "Lead", kind: reflect.Int},
		{name: "Days", kind: reflect.Int},
		{name: "RowCount", kind: reflect.Int},
		{name: "Weekdays", kind: reflect.Slice, typeName: "Weekday"},
		{name: "Rows", kind: reflect.Slice, typeName: "WeekRow"},
		{name: "Intercalary", kind: reflect.Slice, typeName: "IntercalaryDay"},
		{name: "TodayDay", kind: reflect.Int},
		// r51: the grid draws at most three moons; this states how many the
		// calendar DECLARES, so a fourth does not vanish silently.
		{name: "MoonsDeclared", kind: reflect.Int},
		// r53: the Almanac register — every DECLARED moon over this month, at
		// full width and deliberately uncapped by MoonCap. It is the second
		// half of L21: the grid's three-moon ceiling is only legitimate
		// because the overflow body goes somewhere, and this is that
		// somewhere. A rename here silently empties the Shelf's Almanac tab
		// while every hand-written widget fixture stays green.
		{name: "Almanac", kind: reflect.Slice, typeName: "AlmanacMoon"},
	})
}

// TestAlmanacMoon_ShapePinned pins r53's first new type. Every field is written
// by a producer in package calendar and read by a renderer in this package, so
// nothing but reflection would notice a rename — the same argument that gave
// Mark its first pin at r52.
func TestAlmanacMoon_ShapePinned(t *testing.T) {
	assertShape(t, reflect.TypeOf(AlmanacMoon{}), []pinnedField{
		{name: "Name", kind: reflect.String},
		{name: "PeriodDays", kind: reflect.Float64},
		// Drawn is the ONE place the renderer learns which bodies the grid's
		// ceiling excluded. Re-deriving it by counting DayCell.Moons would put
		// moonCap's arithmetic in two places.
		{name: "Drawn", kind: reflect.Bool},
		// The printed arithmetic — "no date in the register was typed by
		// hand". Both are computed against the month's REAL day count, never
		// the mockup's hardcoded thirty.
		{name: "TurnsThisMonth", kind: reflect.Int},
		{name: "DriftDays", kind: reflect.Float64},
		{name: "Days", kind: reflect.Slice, typeName: "AlmanacDay"},
		{name: "NextNewDay", kind: reflect.Int},
		{name: "NextFullDay", kind: reflect.Int},
	})
	// There is deliberately NO Epithet ([S6]): calendar.Moon has no such
	// column and the mockup's "the great pale moon" is fixture text. assertShape
	// already fails on an EXTRA field, so this pin is what refuses the dead
	// column as well as the rename.
}

// TestAlmanacDay_ShapePinned pins r53's second new type. Day is the ordinal the
// data-day ANSWER key is built from (dayKey => "<slug>-<day>"), which is why it
// is an int here and formatted exactly once, in the renderer.
func TestAlmanacDay_ShapePinned(t *testing.T) {
	assertShape(t, reflect.TypeOf(AlmanacDay{}), []pinnedField{
		{name: "Day", kind: reflect.Int},
		{name: "Illum", kind: reflect.Float64},
		{name: "Phase", kind: reflect.String},
		{name: "Turn", kind: reflect.String},
		{name: "Node", kind: reflect.Bool},
	})
}

func TestWeekRow_ShapePinned(t *testing.T) {
	assertShape(t, reflect.TypeOf(WeekRow{}), []pinnedField{
		{name: "Index", kind: reflect.Int},
		// Era bands are PER WEEK ROW — an era spanning three rows cannot be one
		// band. Changing Bands to a month-level field is the regression this
		// entry exists to catch.
		{name: "Bands", kind: reflect.Slice, typeName: "EraBand"},
		{name: "Cells", kind: reflect.Slice, typeName: "DayCell"},
	})
}

func TestDayCell_ShapePinned(t *testing.T) {
	assertShape(t, reflect.TypeOf(DayCell{}), []pinnedField{
		{name: "Day", kind: reflect.Int},
		{name: "Col", kind: reflect.Int},
		{name: "Half", kind: reflect.Bool},
		{name: "IsToday", kind: reflect.Bool},
		{name: "Intercalary", kind: reflect.Bool},
		{name: "Moons", kind: reflect.Slice, typeName: "MoonDisc"},
		{name: "Marks", kind: reflect.Slice, typeName: "Mark"},
		{name: "MoreCount", kind: reflect.Int},
		{name: "Tied", kind: reflect.Bool},
		// Fogged exists so W-F does not have to re-touch every cell. Wave-1
		// producers leave it false; the field must survive to W-F regardless.
		{name: "Fogged", kind: reflect.Bool},
	})
}

// TestMark_ShapePinned. Mark had NO reflect-shape pin before r52, which is
// exactly how a field gets silently dropped by the next amendment: the two
// fields r52 adds are read by a renderer in this package and written by a
// producer in another, and nothing else would notice a rename.
func TestMark_ShapePinned(t *testing.T) {
	assertShape(t, reflect.TypeOf(Mark{}), []pinnedField{
		{name: "EventID", kind: reflect.String},
		{name: "Title", kind: reflect.String},
		{name: "Axis", kind: reflect.String},
		{name: "Pattern", kind: reflect.String},
		{name: "Glyph", kind: reflect.String},
		{name: "Named", kind: reflect.Bool},
		// r51: ink, never membership. The producer emits the whole
		// viewer-visible set in BOTH tie modes and flags each mark.
		{name: "Tied", kind: reflect.Bool},
		// r52: PRE-FORMATTED in the producer, because the calendar's own
		// hour/minute geometry and the viewer's zone live there and this
		// package is plugin-agnostic. Empty DROPS the .tm segment.
		{name: "Time", kind: reflect.String},
		// r52: the event TYPE's display name — the Ledger meta line's only
		// segment in wave 2. Empty DROPS the segment; it is never printed as a
		// dangling separator.
		{name: "AxisLabel", kind: reflect.String},
		{name: "Audience", kind: reflect.Ptr, typeName: "AudienceMark"},
	})
}

func TestSyncPill_ShapePinned(t *testing.T) {
	assertShape(t, reflect.TypeOf(SyncPill{}), []pinnedField{
		{name: "State", kind: reflect.String},
		// Full and Compact are BOTH emitted; CSS container queries choose. A
		// later hand collapsing them to one string breaks the tier rule.
		{name: "Full", kind: reflect.String},
		{name: "Compact", kind: reflect.String},
		{name: "Linked", kind: reflect.Int},
		{name: "Total", kind: reflect.Int},
		{name: "Transport", kind: reflect.String},
		{name: "PushedAgo", kind: reflect.String},
	})
}

// TestLayerState_ShapePinned — r54. LayerState had no shape pin until W-F
// added the field the switchboard needs, which is exactly the moment a struct
// stops being obvious enough to leave unpinned.
//
// THREE FIELDS AND NO FOURTH. The pin is as much a refusal as a record: the
// amendment's §5 turned down eight candidates by name, and two of them would
// have landed here. `Sky` is refused because three of its four values ARE layer
// keys already (graph = moongraph on, moons = moons on, off = neither) and the
// fourth, `words`, names a register that exists nowhere in Chronicle. A
// per-layer `NeedsBackend` flag is refused because two of the three chipped
// zones are filled in this same slice and the third keeps an unconditional
// chip — a flag would be a speculative field, and golangci's `unused` turns
// one of those into a red build rather than a comment.
func TestLayerState_ShapePinned(t *testing.T) {
	assertShape(t, reflect.TypeOf(LayerState{}), []pinnedField{
		// The viewer's OWN set when they have one, the host's seed when they do
		// not. The producer resolves it; the widget never queries.
		{name: "Enabled", kind: reflect.Slice},
		{name: "HasSwitchboard", kind: reflect.Bool},
		// r54. The campaign-scoped endpoint the switchboard posts to, built by
		// the producer because this package has no router and renders under
		// context.Background(). There is deliberately no CSRF field beside it.
		{name: "PersistURL", kind: reflect.String},
	})
}

// TestLayerState_SwitchboardAndURLAreOneFact pins r54's INVARIANT rather than
// leaving it as a comment:
//
//	HasSwitchboard == (PersistURL != "")
//
// It matters because the two ways to break it are opposite and BOTH are silent.
// A true flag with an empty URL renders a live-looking switchboard whose every
// row posts back to the page it is on — a control that looks enabled and does
// nothing, which is exactly the inert-control shape WG-spec V18 forbids. A URL
// with a false flag is a producer that built the endpoint and then left the
// invoker disabled: the only symptom is a missing feature, so nobody ever files
// it.
//
// layerStateConsistent is the predicate, and it is exercised in BOTH directions
// — the two legal states pass, both broken states fail — so the pin cannot be
// satisfied by a checker that always says yes.
func TestLayerState_SwitchboardAndURLAreOneFact(t *testing.T) {
	const url = "/campaigns/camp-1/calendar/prefs"

	for _, tc := range []struct {
		name string
		l    LayerState
		want bool
	}{
		{"no store: the wave-1/2 state, and an anonymous viewer's",
			LayerState{Enabled: []string{"moons"}}, true},
		{"live switchboard",
			LayerState{Enabled: []string{"moons"}, HasSwitchboard: true, PersistURL: url}, true},
		{"flag without an endpoint — rows that post to the page they are on",
			LayerState{Enabled: []string{"moons"}, HasSwitchboard: true}, false},
		{"endpoint without a flag — a switchboard nobody can open",
			LayerState{Enabled: []string{"moons"}, PersistURL: url}, false},
	} {
		if got := layerStateConsistent(tc.l); got != tc.want {
			t.Errorf("%s: consistent=%v, want %v (HasSwitchboard=%v, PersistURL=%q)",
				tc.name, got, tc.want, tc.l.HasSwitchboard, tc.l.PersistURL)
		}
	}

	// The package fixture is a producer's output in miniature and obeys it too.
	if !layerStateConsistent(fullyPopulated().Layers) {
		t.Error("fullyPopulated() emits a LayerState that breaks r54's invariant")
	}
}

// layerStateConsistent is r54's invariant as a predicate. The producers'
// obedience to it is asserted where the producers live (internal/plugins/
// calendar); this is the definition both sides read.
func layerStateConsistent(l LayerState) bool {
	return l.HasSwitchboard == (l.PersistURL != "")
}

func TestViewerContext_ShapePinned(t *testing.T) {
	assertShape(t, reflect.TypeOf(ViewerContext{}), []pinnedField{
		{name: "IsGM", kind: reflect.Bool},
		{name: "UserID", kind: reflect.String},
		{name: "HostEntity", kind: reflect.String},
		// TiedCount and WholeCount must come from THE SAME viewer-filtered pass
		// or the tie toggle becomes an oracle. The pin holds the pair together.
		{name: "TiedCount", kind: reflect.Int},
		{name: "WholeCount", kind: reflect.Int},
		{name: "TieMode", kind: reflect.String},
		{name: "Zone", kind: reflect.String},
		// r52: GM-ONLY BY CONSTRUCTION. The producer sets it only when IsGM, it
		// comes from the same single viewer-filtered pass as the tie pair, and
		// it is never rendered to a player in any form — not even a zero.
		// Pinned here so a later hand cannot quietly move it onto BlockData,
		// where "populate only for a GM" stops being a per-viewer statement.
		{name: "HiddenCount", kind: reflect.Int},
	})
}

// fullyPopulated is the compile-and-shape fixture: every exported field of every
// pinned type carries a non-zero value. It doubles as the compile check — a
// renamed field breaks this literal at build time, before any test runs.
func fullyPopulated() BlockData {
	mark := Mark{
		EventID: "ev-909",
		Title:   "Feast of the Long Dusk",
		Axis:    "a3",
		Pattern: "p4",
		Glyph:   "◆",
		Named:   true,
		Tied:    true,
		// r52. Time carries the zone label already folded in by the producer —
		// there is deliberately no per-mark real-world flag, because whether a
		// time is real-world is a property of the CALENDAR (BlockData
		// .IsRealWorld) and a second copy of one fact can disagree with itself.
		Time:      "18:30 CST",
		AxisLabel: "Festival",
		Audience: &AudienceMark{
			Label:      "GM only",
			Restricted: true,
		},
	}

	cell := DayCell{
		Day:         9,
		Col:         4,
		Half:        true,
		IsToday:     true,
		Intercalary: true,
		Moons: []MoonDisc{{
			Name:       "Selune",
			Illum:      0.62,
			Waxing:     true,
			Eclipse:    true,
			Terminator: "M12,0 A8,12 0 0 1 12,24",
		}},
		Marks:     []Mark{mark},
		MoreCount: 3,
		Tied:      true,
		Fogged:    true, // fixture only — wave-1 producers leave this false
	}

	return BlockData{
		CalendarID:   "cal-42",
		CalendarSlug: "harptos",
		Name:         "Calendar of Harptos",
		CalHue:       "harptos",
		Pattern:      "p2",
		Letter:       "H",
		IsRealWorld:  true,
		IsDefault:    true,
		IsActive:     true,

		DateLabel:   "Sithrel 9, Cycle 218",
		SeasonLabel: "Late Winter",
		EraLabel:    "Age of Ash",
		Fault:       "Needs eras — 0 eras defined, dates cannot resolve",

		Month: MonthGeometry{
			Index:    3,
			Year:     1492,
			Name:     "Sithrel",
			WeekLen:  10,
			Lead:     2,
			Days:     30,
			RowCount: 4,
			Weekdays: []Weekday{{
				Name:  "Firstday",
				Abbr:  "Fi",
				Half:  true,
				Index: 0,
			}},
			Rows: []WeekRow{{
				// Row 1, not row 0: the zero-check below requires every pinned
				// field to be non-zero, and a real row 0 would defeat it.
				Index: 1,
				Bands: []EraBand{{
					Label:     "Age of Ash",
					Suffix:    "Late Winter",
					StartCol:  1,
					Span:      6,
					BandHue:   "b2",
					OpenLeft:  true,
					OpenRight: true,
					Edge:      true,
					Half:      true,
				}},
				Cells: []DayCell{cell},
			}},
			Intercalary: []IntercalaryDay{{
				Name:  "Shieldmeet",
				Day:   31,
				Marks: []Mark{mark},
			}},
			TodayDay: 9, MoonsDeclared: 4,
			// r53: the Almanac register. One lane, fully populated — the
			// register's own contract is "never partially filled", so a
			// fixture entry with an empty Days list would exercise a shape the
			// producer may not emit.
			Almanac: []AlmanacMoon{{
				Name: "Alder", PeriodDays: 31.4, Drawn: true,
				TurnsThisMonth: 1, DriftDays: 30,
				NextNewDay: 3, NextFullDay: 19,
				Days: []AlmanacDay{{Day: 1, Illum: .62, Phase: "waxing gibbous", Turn: "full", Node: true}},
			}},
		},

		Sync: SyncPill{
			State:     "ok",
			Full:      "In sync · 1 of 4 linked · Foundry · pushed 2m ago",
			Compact:   "In sync · 1 of 4",
			Linked:    1,
			Total:     4,
			Transport: "Foundry",
			PushedAgo: "pushed 2m ago",
		},
		Layers: LayerState{
			Enabled:        []string{"moons"},
			HasSwitchboard: true,
			// r54: HasSwitchboard and PersistURL are ONE fact spelled two ways.
			// The fixture sets them together because the invariant below says
			// they can never disagree.
			PersistURL: "/campaigns/camp-1/calendar/prefs",
		},
		Ledger: LedgerStub{NeedsBackend: true, Hidden: true},
		Shelf:  ShelfStub{NeedsBackend: true, Hidden: true},

		Viewer: ViewerContext{
			IsGM:       true,
			UserID:     "u-7",
			HostEntity: "ent-15",
			TiedCount:  4,
			WholeCount: 11,
			TieMode:    "tied",
			Zone:       "America/New_York",
			// Fixture only — a real producer sets this ONLY for a GM (r52).
			HiddenCount: 3,
		},
	}
}

// TestFixture_LeavesNoPinnedFieldZero keeps fullyPopulated() honest. Without it
// a later hand can add a field to a pinned struct, extend the shape pin, and
// leave the fixture untouched — at which point the fixture no longer proves the
// field is constructible or reachable. Walking with reflect means the check
// extends itself.
func TestFixture_LeavesNoPinnedFieldZero(t *testing.T) {
	d := fullyPopulated()

	checks := []struct {
		label string
		val   reflect.Value
	}{
		{"BlockData", reflect.ValueOf(d)},
		{"MonthGeometry", reflect.ValueOf(d.Month)},
		{"WeekRow", reflect.ValueOf(d.Month.Rows[0])},
		{"DayCell", reflect.ValueOf(d.Month.Rows[0].Cells[0])},
		{"Mark", reflect.ValueOf(d.Month.Rows[0].Cells[0].Marks[0])},
		{"AlmanacMoon", reflect.ValueOf(d.Month.Almanac[0])},
		{"AlmanacDay", reflect.ValueOf(d.Month.Almanac[0].Days[0])},
		{"SyncPill", reflect.ValueOf(d.Sync)},
		{"ViewerContext", reflect.ValueOf(d.Viewer)},
		// ADDED WITH r54's PersistURL. LayerState was absent from this walk
		// while it had two fields nobody could forget; the moment it grew a
		// third — one a producer can plausibly leave empty — the fixture needed
		// the same guard every other pinned struct already had.
		{"LayerState", reflect.ValueOf(d.Layers)},
	}

	for _, c := range checks {
		typ := c.val.Type()
		for i := 0; i < typ.NumField(); i++ {
			if !typ.Field(i).IsExported() {
				continue
			}
			if c.val.Field(i).IsZero() {
				t.Errorf("%s.%s is zero in fullyPopulated() — the fixture must exercise every pinned field",
					c.label, typ.Field(i).Name)
			}
		}
	}
}
