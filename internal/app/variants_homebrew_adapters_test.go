package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/systemstate"
)

// fakeVariantPages maps a page to its campaign; fail makes every lookup a
// database failure.
type fakeVariantPages struct {
	campaign map[string]string
	fail     bool
}

func (f *fakeVariantPages) EntityCampaignID(_ context.Context, id string) (string, error) {
	if f.fail {
		return "", apperror.NewInternal(errors.New("db down"))
	}
	c, ok := f.campaign[id]
	if !ok {
		return "", apperror.NewNotFound("entity not found")
	}
	return c, nil
}

// fakeVariantStates serves the gm half of each page's variant record, scoped
// to the campaign as the real service is.
type fakeVariantStates struct {
	pages *fakeVariantPages
	gm    map[string]string
}

func (f *fakeVariantStates) Get(ctx context.Context, campaignID, entityID, systemID, key string) (systemstate.State, error) {
	if systemID != "drawsteel" || key != variantKey {
		return systemstate.State{}, fmt.Errorf("unexpected read %s/%s", systemID, key)
	}
	owner, err := f.pages.EntityCampaignID(ctx, entityID)
	if err != nil {
		return systemstate.State{}, err
	}
	if owner != campaignID {
		return systemstate.State{}, apperror.NewNotFound("entity not found")
	}
	gm, ok := f.gm[entityID]
	if !ok {
		gm = `{}`
	}
	return systemstate.State{GM: json.RawMessage(gm), Public: json.RawMessage(`{}`)}, nil
}

func variantService(t *testing.T, system string, fail bool) homebrew.Service {
	t.Helper()
	pages := &fakeVariantPages{fail: fail, campaign: map[string]string{
		"base1": "camp", "base2": "camp", "addon1": "camp", "addon2": "camp",
		"plain": "camp", "hound": "camp", "wolf": "camp", "var1": "camp",
		"elsewhere": "other",
	}}
	states := &fakeVariantStates{pages: pages, gm: map[string]string{
		"base1":     `{"v":1,"recipe":{"name":"Rem","part":"base","steps":{}}}`,
		"base2":     `{"v":1,"recipe":{"name":"Old","steps":{}}}`,
		"addon1":    `{"v":1,"recipe":{"name":"Rem","part":"addon","label":"Strong"}}`,
		"addon2":    `{"v":1,"recipe":{"name":"Rem","part":"addon","label":"Clever"}}`,
		"hound":     `{"v":1,"follow":{"of":"wolf"}}`,
		"elsewhere": `{"v":1,"recipe":{"part":"base"}}`,
	}}
	reg := homebrew.NewRegistry()
	if err := registerVariantParts(reg, states, pages, func(context.Context, string) string { return system }); err != nil {
		t.Fatal(err)
	}
	return homebrew.NewService(&memRecipes{stored: map[string]homebrew.Recipe{}}, reg, nil)
}

func TestRegisterVariantParts(t *testing.T) {
	reg := homebrew.NewRegistry()
	none := func(context.Context, string) string { return "" }
	if err := registerVariantParts(reg, nil, nil, none); err != nil {
		t.Fatal(err)
	}
	cat := reg.Catalog()
	var got []string
	for _, p := range cat.Parts {
		got = append(got, p.Name+":"+p.ForceSee)
	}
	for _, e := range cat.Events {
		got = append(got, e.Name+"@"+strings.Join(e.PartKinds, ","))
	}
	for _, a := range cat.Actions {
		got = append(got, a.Name+"@"+strings.Join(a.PartKinds, ","))
	}
	want := "creature-mark:director creature-recipe:director creature.dies@creature-mark rise-as-variant@creature-mark"
	if strings.Join(got, " ") != want {
		t.Errorf("catalog = %v, want %s", got, want)
	}
	if err := registerVariantParts(reg, nil, nil, none); err == nil {
		t.Error("registering twice succeeded")
	}
}

func TestVariantPartChecks(t *testing.T) {
	many := func(n int) string {
		ids := make([]string, n)
		for i := range ids {
			ids[i] = fmt.Sprintf(`"p%d"`, i)
		}
		return strings.Join(ids, ",")
	}
	tests := []struct {
		name   string
		system string
		fail   bool
		kind   string
		data   string
		want   string // normalised data; "" means refused with 422; "500" means an internal error
	}{
		{"recipe link to a base", "drawsteel", false, "creature-recipe", `{"page":"base1"}`, `{"page":"base1"}`},
		{"recipe link to an add-on", "drawsteel", false, "creature-recipe", `{"page":"addon1"}`, `{"page":"addon1"}`},
		{"recipe link with no part reads as base", "drawsteel", false, "creature-recipe", `{"page":"base2"}`, `{"page":"base2"}`},
		{"recipe link to a plain page", "drawsteel", false, "creature-recipe", `{"page":"plain"}`, ""},
		{"recipe link to a variant", "drawsteel", false, "creature-recipe", `{"page":"hound"}`, ""},
		{"recipe link to another campaign", "drawsteel", false, "creature-recipe", `{"page":"elsewhere"}`, ""},
		{"recipe link to a missing page", "drawsteel", false, "creature-recipe", `{"page":"nope"}`, ""},
		{"recipe link with no page", "drawsteel", false, "creature-recipe", `{}`, ""},
		{"recipe link with an unknown field", "drawsteel", false, "creature-recipe", `{"page":"base1","steps":{}}`, ""},
		{"recipe link with no game system", "", false, "creature-recipe", `{"page":"base1"}`, ""},
		{"recipe link when the database fails", "drawsteel", true, "creature-recipe", `{"page":"base1"}`, "500"},

		{"mark with everything", "drawsteel", false, "creature-mark",
			`{"recipe":"base1","addons":["addon1","addon2"],"marked":["wolf","plain"],"variants":{"wolf":"hound"}}`,
			`{"recipe":"base1","addons":["addon1","addon2"],"marked":["wolf","plain"],"variants":{"wolf":"hound"}}`},
		{"mark with only a recipe", "drawsteel", false, "creature-mark", `{"recipe":"base1"}`, `{"recipe":"base1"}`},
		{"mark on an add-on recipe", "drawsteel", false, "creature-mark", `{"recipe":"addon1"}`, ""},
		{"mark with a base as an add-on", "drawsteel", false, "creature-mark", `{"recipe":"base1","addons":["base2"]}`, ""},
		{"mark with an add-on twice", "drawsteel", false, "creature-mark", `{"recipe":"base1","addons":["addon1","addon1"]}`, ""},
		{"mark with seven add-ons", "drawsteel", false, "creature-mark", `{"recipe":"base1","addons":[` + many(7) + `]}`, ""},
		{"mark on a creature elsewhere", "drawsteel", false, "creature-mark", `{"recipe":"base1","marked":["elsewhere"]}`, ""},
		{"mark on a missing creature", "drawsteel", false, "creature-mark", `{"recipe":"base1","marked":["nope"]}`, ""},
		{"mark on a creature twice", "drawsteel", false, "creature-mark", `{"recipe":"base1","marked":["wolf","wolf"]}`, ""},
		{"mark on an empty id", "drawsteel", false, "creature-mark", `{"recipe":"base1","marked":[""]}`, ""},
		{"mark on 201 creatures", "drawsteel", false, "creature-mark", `{"recipe":"base1","marked":[` + many(201) + `]}`, ""},
		{"variant for an unmarked creature", "drawsteel", false, "creature-mark", `{"recipe":"base1","marked":["wolf"],"variants":{"plain":"hound"}}`, ""},
		{"variant that is elsewhere", "drawsteel", false, "creature-mark", `{"recipe":"base1","marked":["wolf"],"variants":{"wolf":"elsewhere"}}`, ""},
		{"mark with an unknown field", "drawsteel", false, "creature-mark", `{"recipe":"base1","see":"party"}`, ""},
		{"mark when the database fails", "drawsteel", true, "creature-mark", `{"recipe":"base1"}`, "500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := variantService(t, tt.system, tt.fail)
			p := homebrew.Part{Key: "p", Kind: tt.kind, Label: "P", See: homebrew.SeeParty, Data: json.RawMessage(tt.data)}
			r, err := saveDoc(t, svc, homebrew.Doc{Parts: []homebrew.Part{p}})
			switch tt.want {
			case "":
				if hbCode(err) != 422 {
					t.Fatalf("err = %v, want 422", err)
				}
				return
			case "500":
				if hbCode(err) != 500 {
					t.Fatalf("err = %v, want 500", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := string(r.Doc.Parts[0].Data); got != tt.want {
				t.Errorf("data = %s, want %s", got, tt.want)
			}
			if r.Doc.Parts[0].See != "director" {
				t.Errorf("see = %q, want director", r.Doc.Parts[0].See)
			}
		})
	}
}

func TestVariantRuleChecks(t *testing.T) {
	mark := homebrew.Part{Key: "remnant", Kind: "creature-mark", Label: "Remnant", Data: json.RawMessage(`{"recipe":"base1"}`)}
	link := homebrew.Part{Key: "book", Kind: "creature-recipe", Label: "Book", Data: json.RawMessage(`{"page":"base1"}`)}
	rule := func(eventPart, eventArgs, actPart, actArgs string) homebrew.Rule {
		r := homebrew.Rule{When: homebrew.When{Event: "creature.dies", Part: eventPart}, Timing: "encounter_end",
			Then: []homebrew.Do{{Action: "rise-as-variant", Part: actPart}}}
		if eventArgs != "" {
			r.When.Args = json.RawMessage(eventArgs)
		}
		if actArgs != "" {
			r.Then[0].Args = json.RawMessage(actArgs)
		}
		return r
	}
	tests := []struct {
		name string
		rule homebrew.Rule
		ok   bool
	}{
		{"dies then rises", rule("remnant", "", "remnant", ""), true},
		{"empty args accepted", rule("remnant", `{}`, "remnant", `{}`), true},
		{"dies refuses args", rule("remnant", `{"entityId":"wolf"}`, "remnant", ""), false},
		{"rise refuses args", rule("remnant", "", "remnant", `{"kind":"strong"}`), false},
		{"dies only on a mark", rule("book", "", "remnant", ""), false},
		{"rise only on a mark", rule("remnant", "", "book", ""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := variantService(t, "drawsteel", false)
			_, err := saveDoc(t, svc, homebrew.Doc{Parts: []homebrew.Part{mark, link}, Rules: []homebrew.Rule{tt.rule}})
			if tt.ok && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !tt.ok && hbCode(err) != 422 {
				t.Fatalf("err = %v, want 422", err)
			}
		})
	}
}
