package trackers

import (
	"reflect"
	"testing"
	"time"
)

func ip(n int) *int { return &n }

func add(n int) Entry { return Entry{Op: OpAdd, Amount: n} }
func set(n int) Entry { return Entry{Op: OpSet, Amount: n} }
func struckEntry(e Entry) Entry {
	t := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	e.StruckAt = &t
	return e
}

var (
	kCount     = Kind{Shape: ShapeCount}
	kCountMin  = Kind{Shape: ShapeCount, Min: ip(-3)}
	kCountMax  = Kind{Shape: ShapeCount, Max: ip(10)}
	kCountBoth = Kind{Shape: ShapeCount, Min: ip(1), Max: ip(5)}
	kSteps     = Kind{Shape: ShapeSteps, Steps: []Step{{Name: "a"}, {Name: "b"}, {Name: "c"}}}
	kClock     = Kind{Shape: ShapeClock, Max: ip(6)}
)

func TestBounds(t *testing.T) {
	tests := []struct {
		name   string
		k      Kind
		lo, hi int
		hasMax bool
	}{
		{"count unbounded", kCount, 0, 0, false},
		{"count min only", kCountMin, -3, 0, false},
		{"count max only", kCountMax, 0, 10, true},
		{"count both", kCountBoth, 1, 5, true},
		{"steps", kSteps, 0, 2, true},
		{"clock", kClock, 0, 6, true},
		{"clock without max", Kind{Shape: ShapeClock}, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lo, hi, hasMax := bounds(tt.k)
			if lo != tt.lo || hi != tt.hi || hasMax != tt.hasMax {
				t.Fatalf("bounds = %d,%d,%v want %d,%d,%v", lo, hi, hasMax, tt.lo, tt.hi, tt.hasMax)
			}
		})
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		name string
		k    Kind
		in   int
		want int
	}{
		{"unbounded count goes negative", kCount, -50, -50},
		{"count with only a max has no floor", kCountMax, -7, -7},
		{"unbounded count goes high", kCount, 5000, 5000},
		{"min floor", kCountMin, -9, -3},
		{"max ceiling", kCountMax, 99, 10},
		{"inside", kCountBoth, 3, 3},
		{"below both", kCountBoth, 0, 1},
		{"above both", kCountBoth, 6, 5},
		{"steps below", kSteps, -1, 0},
		{"steps above", kSteps, 3, 2},
		{"clock below", kClock, -1, 0},
		{"clock above", kClock, 7, 6},
		{"clock top", kClock, 6, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clamp(tt.k, tt.in); got != tt.want {
				t.Fatalf("clamp = %d want %d", got, tt.want)
			}
		})
	}
}

func TestFold(t *testing.T) {
	tests := []struct {
		name      string
		k         Kind
		entries   []Entry
		want      int
		wantAfter []int
	}{
		{"no log starts at the floor", kCountBoth, nil, 1, []int{}},
		{"no log unbounded starts at zero", kCount, nil, 0, []int{}},
		{"unbounded count can go negative", kCount, []Entry{add(-2), add(-1)}, -3, []int{-2, -3}},
		{"max below zero starts inside the bound", Kind{Shape: ShapeCount, Max: ip(-5)}, nil, -5, []int{}},
		{"adds accumulate", kCount, []Entry{add(2), add(3), add(-1)}, 4, []int{2, 5, 4}},
		{"set then add", kCount, []Entry{set(10), add(-3)}, 7, []int{10, 7}},
		{"add then set replaces", kCount, []Entry{add(4), set(1)}, 1, []int{4, 1}},
		{"clamped at the top per line", kCountMax, []Entry{add(8), add(8), add(-1)}, 9, []int{8, 10, 9}},
		{"clamped at the bottom per line, step back is lost", kCountBoth, []Entry{add(-1), add(-1), add(2)}, 3, []int{1, 1, 3}},
		{"set is clamped", kCountBoth, []Entry{set(99), add(-1)}, 4, []int{5, 4}},
		{"struck entry skipped", kCount, []Entry{add(5), struckEntry(add(100)), add(1)}, 6, []int{5, 5, 6}},
		{"struck set skipped", kCount, []Entry{add(5), struckEntry(set(-9))}, 5, []int{5, 5}},
		{"struck first line leaves the start", kCountBoth, []Entry{struckEntry(add(3))}, 1, []int{1}},
		{"steps walk", kSteps, []Entry{add(1), add(1), add(1), add(-2)}, 0, []int{1, 2, 2, 0}},
		{"steps set", kSteps, []Entry{set(2), add(-1)}, 1, []int{2, 1}},
		{"clock fills and stops", kClock, []Entry{add(4), add(4), add(-1)}, 5, []int{4, 6, 5}},
		{"clock cannot go below empty", kClock, []Entry{add(-3), add(2)}, 2, []int{0, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, after := fold(tt.k, tt.entries)
			if got != tt.want {
				t.Fatalf("value = %d want %d", got, tt.want)
			}
			if !reflect.DeepEqual(after, tt.wantAfter) {
				t.Fatalf("after = %v want %v", after, tt.wantAfter)
			}
		})
	}
}

func TestNowText(t *testing.T) {
	stepsKind := Kind{Shape: ShapeSteps, Steps: []Step{
		{Name: "Calm", Text: "Nobody is looking"},
		{Name: "Watched"},
	}}
	bands := Kind{Shape: ShapeCount, Bands: []Band{
		{At: 10, Text: "high"}, {At: 0, Text: "low"}, {At: 5, Text: "mid"},
	}}
	negBands := Kind{Shape: ShapeCount, Bands: []Band{{At: -5, Text: "dread"}, {At: 0, Text: "fine"}}}
	clock := Kind{Shape: ShapeClock, Max: ip(6), Bands: []Band{{At: 3, Text: "half"}, {At: 6, Text: "done"}}}

	tests := []struct {
		name string
		k    Kind
		v    int
		want string
	}{
		{"step uses its text", stepsKind, 0, "Nobody is looking"},
		{"step falls back to name", stepsKind, 1, "Watched"},
		{"step out of range high", stepsKind, 2, ""},
		{"step out of range low", stepsKind, -1, ""},
		{"band exactly at", bands, 5, "mid"},
		{"highest band at or below, unsorted input", bands, 9, "mid"},
		{"above every band", bands, 50, "high"},
		{"lowest band", bands, 0, "low"},
		{"no band below the value", bands, -1, ""},
		{"negative bands", negBands, -2, "dread"},
		{"negative value under all bands", negBands, -6, ""},
		{"no bands at all", kCount, 3, ""},
		{"clock below first band", clock, 2, ""},
		{"clock between bands", clock, 4, "half"},
		{"clock full", clock, 6, "done"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nowText(tt.k, tt.v); got != tt.want {
				t.Fatalf("nowText = %q want %q", got, tt.want)
			}
		})
	}
}
