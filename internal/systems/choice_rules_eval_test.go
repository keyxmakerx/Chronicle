package systems

import (
	"reflect"
	"testing"
)

func pkg(key, name string) Choice {
	return Choice{Key: key, Slug: key, Name: name, Source: ChoiceSourcePackage}
}

func campChoice(key, name string) Choice {
	return Choice{Key: key, Name: name, Source: ChoiceSourceCampaign}
}

func rule(source, key, target, state string) ChoiceRule {
	return ChoiceRule{FieldKey: "ancestry", Source: source, Key: key, Target: target, State: state}
}

func choiceNames(cs []Choice) []string {
	out := []string{}
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func TestApplyChoiceRulesPrecedence(t *testing.T) {
	fury, devil, human := pkg("fury", "Fury"), pkg("devil", "Devil"), pkg("human", "Human")
	moth := campChoice("7", "Mothfolk")
	all := []Choice{devil, fury, human, moth}
	own := campChoice("9", "Pat's Own")
	own.Mine = true

	tests := []struct {
		name      string
		choices   []Choice
		rows      []ChoiceRule
		viewer    string
		kept      []string
		notHere   []string
		required  bool
		prefilled string
		limited   bool
	}{
		{name: "no rules offers everything", choices: all, viewer: "pat",
			kept: []string{"Devil", "Fury", "Human", "Mothfolk"}},
		{name: "a rule about another player is not this player's", choices: all, viewer: "pat",
			rows: []ChoiceRule{rule("package", "fury", "lee", RuleNotOffered)},
			kept: []string{"Devil", "Fury", "Human", "Mothfolk"}},
		{name: "everyone entry ban", choices: all, viewer: "lee",
			rows: []ChoiceRule{rule("package", "fury", "", RuleNotOffered)},
			kept: []string{"Devil", "Human", "Mothfolk"}, notHere: []string{"Fury"}, limited: true},
		{name: "no Furies, except Pat", choices: all, viewer: "pat",
			rows: []ChoiceRule{rule("package", "fury", "", RuleNotOffered), rule("package", "fury", "pat", RuleOpen)},
			kept: []string{"Devil", "Fury", "Human", "Mothfolk"}, limited: true},
		{name: "no Furies, except Pat: someone else", choices: all, viewer: "lee",
			rows: []ChoiceRule{rule("package", "fury", "", RuleNotOffered), rule("package", "fury", "pat", RuleOpen)},
			kept: []string{"Devil", "Human", "Mothfolk"}, notHere: []string{"Fury"}, limited: true},
		{name: "everyone group ban of homebrew", choices: all, viewer: "lee",
			rows: []ChoiceRule{rule("campaign", "*", "", RuleNotOffered)},
			kept: []string{"Devil", "Fury", "Human"}, notHere: []string{"Mothfolk"}, limited: true},
		{name: "no homebrew, except Mothfolk (entry beats group)", choices: all, viewer: "lee",
			rows: []ChoiceRule{rule("campaign", "*", "", RuleNotOffered), rule("campaign", "7", "", RuleAskFirst)},
			kept: []string{"Devil", "Fury", "Human", "Mothfolk"}, limited: true},
		{name: "a player group ban beats an everyone entry allow", choices: all, viewer: "pat",
			rows: []ChoiceRule{rule("campaign", "7", "", RuleOpen), rule("campaign", "*", "pat", RuleNotOffered)},
			kept: []string{"Devil", "Fury", "Human"}, notHere: []string{"Mothfolk"}, limited: true},
		{name: "a player entry allow beats the player's own group ban", choices: all, viewer: "pat",
			rows: []ChoiceRule{rule("campaign", "*", "pat", RuleNotOffered), rule("campaign", "7", "pat", RuleOpen)},
			kept: []string{"Devil", "Fury", "Human", "Mothfolk"}, limited: true},
		{name: "required is a short list", choices: all, viewer: "lee",
			rows: []ChoiceRule{rule("package", "human", "", RuleRequired), rule("package", "devil", "", RuleRequired)},
			kept: []string{"Devil", "Human"}, required: true, limited: true},
		{name: "required short list with a player's extra open", choices: all, viewer: "pat",
			rows: []ChoiceRule{rule("package", "human", "", RuleRequired), rule("package", "devil", "", RuleRequired), rule("package", "fury", "pat", RuleOpen)},
			kept: []string{"Devil", "Fury", "Human"}, required: true, limited: true},
		{name: "one required entry is filled in", choices: all, viewer: "lee",
			rows: []ChoiceRule{rule("package", "human", "", RuleRequired)},
			kept: []string{"Human"}, required: true, prefilled: "Human", limited: true},
		{name: "a player extra open stops the fill-in", choices: all, viewer: "pat",
			rows: []ChoiceRule{rule("package", "human", "", RuleRequired), rule("package", "fury", "pat", RuleOpen)},
			kept: []string{"Fury", "Human"}, required: true, limited: true},
		{name: "a grant is implied open under a required list", choices: append([]Choice{own}, all...), viewer: "pat",
			rows: []ChoiceRule{rule("package", "human", "", RuleRequired)},
			kept: []string{"Human", "Pat's Own"}, required: true, limited: true},
		{name: "a player-level ban beats the implied grant", choices: append([]Choice{own}, all...), viewer: "pat",
			rows: []ChoiceRule{rule("package", "human", "", RuleRequired), rule("campaign", "9", "pat", RuleNotOffered)},
			kept: []string{"Human"}, notHere: []string{"Pat's Own"}, required: true, prefilled: "Human", limited: true},
		{name: "the grant beats an everyone group ban", choices: append([]Choice{own}, all...), viewer: "pat",
			rows: []ChoiceRule{rule("campaign", "*", "", RuleNotOffered)},
			kept: []string{"Pat's Own", "Devil", "Fury", "Human"}, notHere: []string{"Mothfolk"}, limited: true},
		{name: "a player ban on the only required entry opens the step", choices: all, viewer: "pat",
			rows: []ChoiceRule{rule("package", "human", "", RuleRequired), rule("package", "human", "pat", RuleNotOffered)},
			kept: []string{"Devil", "Fury", "Mothfolk"}, notHere: []string{"Human"}, limited: true},
		{name: "ask first stays on the list marked", choices: all, viewer: "lee",
			rows: []ChoiceRule{rule("package", "fury", "", RuleAskFirst)},
			kept: []string{"Devil", "Fury", "Human", "Mothfolk"}, limited: true},
		{name: "a player ban leaves one required entry, filled in", choices: all, viewer: "lee",
			rows: []ChoiceRule{rule("package", "fury", "", RuleRequired), rule("package", "human", "", RuleRequired), rule("package", "human", "lee", RuleNotOffered)},
			kept: []string{"Fury"}, notHere: []string{"Human"}, required: true, prefilled: "Fury", limited: true},
		{name: "no account sees only everyone's rows", choices: all, viewer: "",
			rows: []ChoiceRule{rule("package", "fury", "", RuleNotOffered), rule("package", "devil", "", RuleOpen)},
			kept: []string{"Devil", "Human", "Mothfolk"}, notHere: []string{"Fury"}, limited: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyChoiceRules(tt.choices, tt.rows, tt.viewer)
			if !reflect.DeepEqual(choiceNames(got.Kept), tt.kept) && !sameSet(choiceNames(got.Kept), tt.kept) {
				t.Errorf("kept = %v, want %v", choiceNames(got.Kept), tt.kept)
			}
			nh := []string{}
			for _, n := range got.NotHere {
				nh = append(nh, n.Name)
			}
			if tt.notHere == nil {
				tt.notHere = []string{}
			}
			if !reflect.DeepEqual(nh, tt.notHere) {
				t.Errorf("notHere = %v, want %v", nh, tt.notHere)
			}
			if got.Required != tt.required || got.Prefilled != tt.prefilled || got.Limited != tt.limited {
				t.Errorf("required/prefilled/limited = %v/%q/%v, want %v/%q/%v",
					got.Required, got.Prefilled, got.Limited, tt.required, tt.prefilled, tt.limited)
			}
		})
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, x := range a {
		m[x]++
	}
	for _, x := range b {
		m[x]--
	}
	for _, n := range m {
		if n != 0 {
			return false
		}
	}
	return true
}

func TestApplyChoiceRulesMarksAskFirst(t *testing.T) {
	got := applyChoiceRules([]Choice{pkg("fury", "Fury"), pkg("devil", "Devil")},
		[]ChoiceRule{rule("package", "fury", "", RuleAskFirst)}, "lee")
	for _, c := range got.Kept {
		want := ""
		if c.Name == "Fury" {
			want = ChoiceStateAskFirst
		}
		if c.State != want {
			t.Errorf("%s state = %q, want %q", c.Name, c.State, want)
		}
	}
}

func TestApplyChoiceRulesKeepsExcludedData(t *testing.T) {
	fury := pkg("fury", "Fury")
	fury.Properties = map[string]any{"points": float64(3)}
	got := applyChoiceRules([]Choice{fury}, []ChoiceRule{rule("package", "fury", "", RuleNotOffered)}, "lee")
	if len(got.Excluded) != 1 || got.Excluded[0].Properties["points"] != float64(3) {
		t.Fatalf("excluded = %+v, want Fury with its data so a hero holding it stays editable", got.Excluded)
	}
}

func TestFieldRulesNotHereCountAndReasons(t *testing.T) {
	var cs []Choice
	var rows []ChoiceRule
	for i := 0; i < 150; i++ {
		k := string(rune('a'+i%26)) + string(rune('a'+i/26))
		cs = append(cs, pkg(k, "E"+k))
		r := rule("package", k, "", RuleNotOffered)
		r.Reason = "too strong"
		rows = append(rows, r)
	}
	fr := applyChoiceRules(cs, rows, "lee").fieldRules()
	if fr == nil || fr.NotHereCount != 150 || len(fr.NotHere) != maxNotHereNames || fr.NotHere[0].Reason != "too strong" {
		t.Fatalf("rules = %+v, want the full count, a capped name list and the reason", fr)
	}
	if applyChoiceRules(cs, nil, "lee").fieldRules() != nil {
		t.Fatal("a viewer no rule applies to gets no rules summary")
	}
}

func TestBreakFor(t *testing.T) {
	list := []Choice{pkg("fury", "Fury"), pkg("devil", "Devil"), pkg("human", "Human")}
	ban := ChoiceRule{FieldKey: "ancestry", Source: "package", Key: "fury", State: RuleNotOffered, Reason: "no"}
	req := []ChoiceRule{rule("package", "human", "", RuleRequired)}
	tests := []struct {
		name  string
		rows  []ChoiceRule
		value string
		want  string
	}{
		{"no rules", nil, "Fury", ""},
		{"empty value holds nothing", []ChoiceRule{ban}, "", ""},
		{"kept-off entry breaks", []ChoiceRule{ban}, "fury", BreakNotOffered},
		{"another entry is fine", []ChoiceRule{ban}, "Devil", ""},
		{"off the short list breaks", req, "Devil", BreakOffList},
		{"free text off the short list breaks", req, "Something typed", BreakOffList},
		{"on the short list is fine", req, "Human", ""},
		{"free text under a ban only is fine", []ChoiceRule{ban}, "Something typed", ""},
		{"an ask-first entry the hero holds counts as allowed", []ChoiceRule{rule("package", "fury", "", RuleAskFirst)}, "Fury", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := breakFor(list, tt.rows, "pat", "ancestry", tt.value)
			switch {
			case tt.want == "" && got != nil:
				t.Fatalf("break = %+v, want none", got)
			case tt.want != "" && (got == nil || got.Kind != tt.want):
				t.Fatalf("break = %+v, want kind %s", got, tt.want)
			}
		})
	}
	if b := breakFor(list, []ChoiceRule{ban}, "pat", "ancestry", "Fury"); b == nil || b.Reason != "no" {
		t.Fatalf("break = %+v, want the Director's reason carried", b)
	}
}

// A rewrite takes the standard entry's place, so a rule the Director set on
// the standard entry still holds for it; a rule on the rewrite itself wins.
func TestApplyChoiceRulesFollowRewrite(t *testing.T) {
	rewrite := campChoice("12", "Fury (our words)")
	rewrite.Origin, rewrite.StandsFor = OriginRewritten, "fury"
	plain := campChoice("13", "Mothfolk")
	devil := pkg("devil", "Devil")
	all := []Choice{devil, rewrite, plain}
	tests := []struct {
		name    string
		rows    []ChoiceRule
		viewer  string
		kept    []string
		askNext string
	}{
		{name: "not here on the standard entry holds for the rewrite", viewer: "lee",
			rows: []ChoiceRule{rule("package", "fury", "", RuleNotOffered)},
			kept: []string{"Devil", "Mothfolk"}},
		{name: "a player's own opening on the standard entry still lets them", viewer: "pat",
			rows: []ChoiceRule{rule("package", "fury", "", RuleNotOffered), rule("package", "fury", "pat", RuleOpen)},
			kept: []string{"Devil", "Fury (our words)", "Mothfolk"}},
		{name: "a rule on the rewrite itself wins", viewer: "lee",
			rows: []ChoiceRule{rule("package", "fury", "", RuleNotOffered), rule("campaign", "12", "", RuleOpen)},
			kept: []string{"Devil", "Fury (our words)", "Mothfolk"}},
		{name: "ask first follows the rewrite", viewer: "lee",
			rows: []ChoiceRule{rule("package", "fury", "", RuleAskFirst)},
			kept: []string{"Devil", "Fury (our words)", "Mothfolk"}, askNext: "Fury (our words)"},
		{name: "a campaign entry that rewrites nothing is untouched", viewer: "lee",
			rows: []ChoiceRule{rule("package", "13", "", RuleNotOffered)},
			kept: []string{"Devil", "Fury (our words)", "Mothfolk"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := applyChoiceRules(all, tc.rows, tc.viewer)
			if names := choiceNames(got.Kept); !reflect.DeepEqual(names, tc.kept) {
				t.Errorf("kept = %v, want %v", names, tc.kept)
			}
			for _, c := range got.Kept {
				want := ""
				if c.Name == tc.askNext {
					want = ChoiceStateAskFirst
				}
				if c.State != want {
					t.Errorf("%s state = %q, want %q", c.Name, c.State, want)
				}
			}
		})
	}
}
