package rolls

import (
	"reflect"
	"testing"
)

func TestParseFormula(t *testing.T) {
	tests := []struct {
		in    string
		ok    bool
		shown string
		terms []term
	}{
		{"2d10+2", true, "2d10+2", []term{{1, 2, 10}, {1, 2, 0}}},
		{"d20", true, "1d20", []term{{1, 1, 20}}},
		{" 4d6 - 1 ", true, "4d6-1", []term{{1, 4, 6}, {-1, 1, 0}}},
		{"+1d8+1d6", true, "1d8+1d6", []term{{1, 1, 8}, {1, 1, 6}}},
		{"D100", true, "1d100", []term{{1, 1, 100}}},
		{"", false, "", nil},
		{"5", false, "", nil},               // no dice
		{"2d7", false, "", nil},             // no such die at the table
		{"-1d6", false, "", nil},            // dice are never taken away
		{"2d10+", false, "", nil},           // dangling sign
		{"d", false, "", nil},               // no sides
		{"2d10*2", false, "", nil},          // unknown operator
		{"51d6", false, "", nil},            // too many dice
		{"30d6+30d6", false, "", nil},       // too many dice across terms
		{"1d20+10000", false, "", nil},      // modifier out of range
		{"1d20+9999999999", false, "", nil}, // a huge number
		{"0d6", false, "", nil},             // no dice in the term
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			terms, shown, ok := parseFormula(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if shown != tt.shown {
				t.Errorf("shown = %q, want %q", shown, tt.shown)
			}
			if !reflect.DeepEqual(terms, tt.terms) {
				t.Errorf("terms = %v, want %v", terms, tt.terms)
			}
		})
	}
}

func TestThrowAddsDiceAndModifier(t *testing.T) {
	terms, _, _ := parseFormula("2d10+3-1")
	n := 0
	dice, mod, total := throw(terms, func(sides int) int { n++; return n })
	want := []Die{{10, 1}, {10, 2}}
	if !reflect.DeepEqual(dice, want) || mod != 2 || total != 5 {
		t.Fatalf("got dice %v mod %d total %d", dice, mod, total)
	}
}

func TestCryptoRollStaysOnTheDie(t *testing.T) {
	for i := 0; i < 2000; i++ {
		if v := cryptoRoll(6); v < 1 || v > 6 {
			t.Fatalf("rolled %d on a d6", v)
		}
	}
	if cryptoRoll(1) != 1 {
		t.Fatal("a one-sided die shows 1")
	}
}
