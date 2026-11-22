package records

import (
	"context"
	"strings"
	"testing"
)

func TestRulesLookup_Answer(t *testing.T) {
	hb := &fakeHB{
		rules: []HomebrewRule{{ID: 1, Name: "Guild rank", Follows: "echelon", Rows: []RuleRow{{Is: "1", Gives: "Copper"}, {From: fp(4), Gives: "Starsteel"}, {From: fp(2.5), To: fp(3), Gives: "Gold: bright"}}}},
		words: []HomebrewWord{{ID: 1, Word: "Mark", Local: "Grain", PlaceName: "Saltreach", DMOnly: true}, {ID: 2, Word: "Crown", Local: "Sun", EraID: 3, EraName: "Dim Age"}},
	}
	l := RulesLookup{Svc: hb}
	chip, text, err := l.Answer(context.Background(), camp, owner, Record{})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{
		"kind: field-rule\nname: Guild rank\nfollows: echelon\nrows:\n  - is: 1\n    gives: Copper\n  - from: 4\n    gives: Starsteel\n  - from: 2.5\n    to: 3\n    gives: \"Gold: bright\"\n",
		"kind: local-word\nname: Mark\nlocal: Grain\nplace: Saltreach\ndirector: true\n",
		"kind: local-word\nname: Crown\nlocal: Sun\nera: Dim Age\n",
	} {
		if !strings.Contains(text, w) {
			t.Errorf("answer lacks %q:\n%s", w, text)
		}
	}
	if !strings.Contains(chip, "1 rules, 2 words") {
		t.Errorf("chip = %q", chip)
	}
}

// Safe privacy and a player's key read as a player and get nothing, with the
// service never asked; the tool is hidden from a player's key as well.
func TestRulesLookup_DirectorsOnly(t *testing.T) {
	hb := &fakeHB{rules: []HomebrewRule{{Name: "Secret rule", Follows: "x"}}}
	_, text, err := RulesLookup{Svc: hb}.Answer(context.Background(), camp, player, Record{})
	if err == nil || !strings.Contains(err.Error(), "Safe privacy") || strings.Contains(text, "Secret rule") {
		t.Errorf("player got %q, %v", text, err)
	}
	if in := (RulesLookup{}).LookupInfo(); in.What != "rules" || in.Tool.Name != "sheet_rules" || !in.Tool.Owner {
		t.Errorf("info = %+v", in)
	}
}
