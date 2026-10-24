package campaigns

import (
	"slices"
	"testing"
)

// TestShownLookParts pins that "Your look" leaves out the character sheet
// styles only when the campaign has no sheet, asks about the sheet only when
// that part is there, and never changes the list it was given.
func TestShownLookParts(t *testing.T) {
	sheet, _ := LookPartByKey(LookPartSheet)
	dm, _ := LookPartByKey(LookPartDmScreen)
	keys := func(ps []LookPart) []string {
		out := []string{}
		for _, p := range ps {
			out = append(out, p.Key)
		}
		return out
	}
	tests := []struct {
		name  string
		parts []LookPart
		check func(*bool) func() bool
		want  []string
		asked bool
	}{
		{"no sheet drops the part", []LookPart{sheet, dm}, answer(false), []string{LookPartDmScreen}, true},
		{"a sheet keeps it", []LookPart{sheet, dm}, answer(true), []string{LookPartSheet, LookPartDmScreen}, true},
		{"no sheet part, nothing asked", []LookPart{dm}, answer(false), []string{LookPartDmScreen}, false},
		{"no check keeps every part", []LookPart{sheet}, nil, []string{LookPartSheet}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := keys(tc.parts)
			asked := false
			var check func() bool
			if tc.check != nil {
				check = tc.check(&asked)
			}
			got := keys(ShownLookParts(tc.parts, check))
			if !slices.Equal(got, tc.want) {
				t.Fatalf("parts = %v, want %v", got, tc.want)
			}
			if asked != tc.asked {
				t.Fatalf("asked = %v, want %v", asked, tc.asked)
			}
			if !slices.Equal(keys(tc.parts), before) {
				t.Fatalf("input changed to %v", keys(tc.parts))
			}
		})
	}
}

// answer is a sheet check that records it was asked.
func answer(has bool) func(*bool) func() bool {
	return func(asked *bool) func() bool {
		return func() bool { *asked = true; return has }
	}
}
