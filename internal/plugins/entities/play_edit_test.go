package entities

import (
	"encoding/json"
	"strings"
	"testing"
)

// playType is a hero's category with play blocks as a system declares them:
// awarded numbers that ring the bell, a bounded counter, one the GM keeps,
// and an owner block on a field the type marks GM-only.
func playType() []FieldDefinition {
	zero, ten := 0.0, 10.0
	return []FieldDefinition{
		{Key: "victories", Label: "Victories", Play: &FieldPlay{Edit: "owner", Kind: "counter", Min: &zero, Step: 1, Bell: true}},
		{Key: "xp", Label: "XP", Play: &FieldPlay{Edit: "owner", Kind: "counter", Min: &zero, Step: 1, Bell: true}},
		{Key: "stamina_current", Label: "Stamina", Play: &FieldPlay{Edit: "owner", Kind: "counter", MaxField: "stamina_max", Step: 1}},
		{Key: "stamina_max", Label: "Max Stamina"},
		{Key: "surges", Label: "Surges", Play: &FieldPlay{Edit: "owner", Kind: "resource", Min: &zero, Max: &ten}},
		{Key: "level", Label: "Level", Play: &FieldPlay{Edit: "gm", Kind: "counter"}},
		{Key: "doom", Label: "Doom", GMOnly: true, Play: &FieldPlay{Edit: "owner", Kind: "counter"}},
		{Key: "culture", Label: "Culture", Play: &FieldPlay{Edit: "owner", Kind: "text"}},
	}
}

// A claimed owner may write a number the system's play block lets them
// change, within its bounds, and nothing else of the kind.
func TestOwnerFieldVerdicts_PlayNumbers(t *testing.T) {
	e := &Entity{ID: "e1", FieldsData: map[string]any{"victories": float64(2), "stamina_max": float64(18)}}
	cases := []struct {
		name  string
		patch map[string]any
		want  string // refusal text for the one key, "" when it may be written
	}{
		{"a victory added", map[string]any{"victories": float64(3)}, ""},
		{"banked at a respite", map[string]any{"victories": float64(0), "xp": float64(6)}, ""},
		{"a number sent as JSON number", map[string]any{"xp": json.Number("4")}, ""},
		{"cleared", map[string]any{"xp": nil}, ""},
		{"below the min", map[string]any{"victories": float64(-1)}, "Victories can't go below 0"},
		{"a fraction", map[string]any{"xp": 1.5}, "XP takes a whole number"},
		{"words", map[string]any{"xp": "lots"}, "XP takes a number"},
		{"a number as text", map[string]any{"xp": "4"}, "XP takes a number"},
		{"too large", map[string]any{"xp": 1e12}, "XP is too large"},
		{"within the max field", map[string]any{"stamina_current": float64(18)}, ""},
		{"above the max field", map[string]any{"stamina_current": float64(19)}, "Stamina can't go above 18"},
		{"above the literal max", map[string]any{"surges": float64(11)}, "Surges can't go above 10"},
		{"a number the GM keeps", map[string]any{"level": float64(2)}, "your GM's to change"},
		{"a GM-only field whatever its block says", map[string]any{"doom": float64(1)}, "your GM's to change"},
		{"a field with no play block", map[string]any{"stamina_max": float64(30)}, "your GM's to change"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			held := ownerFieldVerdicts(e, tc.patch, playType(), nil, false)
			if tc.want == "" {
				if len(held) != 0 {
					t.Fatalf("refused: %v", held)
				}
				return
			}
			if len(held) != 1 {
				t.Fatalf("want one refusal %q, got %v", tc.want, held)
			}
			for _, err := range held {
				if !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("refusal %q, want it to say %q", err.Error(), tc.want)
				}
			}
		})
	}

	// A max field set in the same write bounds by its new value.
	if held := ownerFieldVerdicts(e, map[string]any{"stamina_current": float64(19), "stamina_max": float64(20)}, playType(), nil, false); held["stamina_current"] != nil {
		t.Fatalf("the patch's own max should bound: %v", held)
	}
	// Without the type the door can't read a play block, so the number stays the GM's.
	if held := ownerFieldVerdicts(e, map[string]any{"victories": float64(3)}, nil, nil, false); held["victories"] == nil {
		t.Fatal("with no type known, a play number must be refused")
	}
}

// A number whose play block asks for the bell rings with its old and new
// values; an unchanged resend, a number without the bell, and filling one for
// the first time from nothing all behave as the table expects.
func TestHeroBellChanges_PlayNumbers(t *testing.T) {
	e := &Entity{ID: "e1", FieldsData: map[string]any{"victories": float64(2), "xp": float64(4), "stamina_current": float64(10)}}
	cases := []struct {
		name  string
		patch map[string]any
		want  string
	}{
		{"a victory added", map[string]any{"victories": float64(3)}, "number Victories 2>3 + -"},
		{"banked at a respite", map[string]any{"victories": float64(0), "xp": float64(6)}, "number Victories 2>0 + -|number XP 4>6 + -"},
		{"sent back unchanged", map[string]any{"victories": float64(2), "xp": float64(4)}, ""},
		{"a number without the bell", map[string]any{"stamina_current": float64(5)}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(bellSummary(heroBellChanges(e, tc.patch, nil, nil, playType())), "|")
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	blank := &Entity{ID: "e2", FieldsData: map[string]any{}}
	if got := strings.Join(bellSummary(heroBellChanges(blank, map[string]any{"victories": float64(1)}, nil, nil, playType())), "|"); got != "number Victories 0>1 + -" {
		t.Fatalf("a first award: got %q", got)
	}
	// A play number is never locked by the Director's window for changing heroes.
	for _, c := range heroBellChanges(e, map[string]any{"victories": float64(3)}, nil, nil, playType()) {
		if c.locked() {
			t.Fatal("a play number must never be locked")
		}
	}
}
