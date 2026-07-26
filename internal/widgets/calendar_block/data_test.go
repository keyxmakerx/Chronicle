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
		{name: "CalendarID", kind: reflect.Int64},
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

func TestViewerContext_ShapePinned(t *testing.T) {
	assertShape(t, reflect.TypeOf(ViewerContext{}), []pinnedField{
		{name: "IsGM", kind: reflect.Bool},
		{name: "UserID", kind: reflect.Int64},
		{name: "HostEntity", kind: reflect.Int64},
		// TiedCount and WholeCount must come from THE SAME viewer-filtered pass
		// or the tie toggle becomes an oracle. The pin holds the pair together.
		{name: "TiedCount", kind: reflect.Int},
		{name: "WholeCount", kind: reflect.Int},
		{name: "TieMode", kind: reflect.String},
		{name: "Zone", kind: reflect.String},
	})
}

// fullyPopulated is the compile-and-shape fixture: every exported field of every
// pinned type carries a non-zero value. It doubles as the compile check — a
// renamed field breaks this literal at build time, before any test runs.
func fullyPopulated() BlockData {
	mark := Mark{
		EventID: 909,
		Title:   "Feast of the Long Dusk",
		Axis:    "a3",
		Pattern: "p4",
		Glyph:   "◆",
		Named:   true,
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
		CalendarID:   42,
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
			TodayDay: 9,
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
			HasSwitchboard: true, // fixture only — wave-1 renders DEF with this false
		},
		Ledger: LedgerStub{NeedsBackend: true, Hidden: true},
		Shelf:  ShelfStub{NeedsBackend: true, Hidden: true},

		Viewer: ViewerContext{
			IsGM:       true,
			UserID:     7,
			HostEntity: 15,
			TiedCount:  4,
			WholeCount: 11,
			TieMode:    "tied",
			Zone:       "America/New_York",
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
		{"SyncPill", reflect.ValueOf(d.Sync)},
		{"ViewerContext", reflect.ValueOf(d.Viewer)},
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
