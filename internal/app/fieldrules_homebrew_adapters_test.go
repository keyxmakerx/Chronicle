package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

// fieldRuleService runs recipes through the real homebrew service with the
// field rule parts registered; "saltreach" is the campaign's only page.
func fieldRuleService(t *testing.T) (homebrew.Service, *homebrew.Registry) {
	t.Helper()
	reg := homebrew.NewRegistry()
	place := func(_ context.Context, campaignID, id string) (string, bool) {
		return "Saltreach", campaignID == "camp" && id == "saltreach"
	}
	if err := registerFieldRuleParts(reg, place, fieldRuleSync{}); err != nil {
		t.Fatal(err)
	}
	return homebrew.NewService(&memRecipes{stored: map[string]homebrew.Recipe{}}, reg, nil, nil), reg
}

func TestRegisterFieldRuleParts(t *testing.T) {
	_, reg := fieldRuleService(t)
	var got []string
	for _, p := range reg.Catalog().Parts {
		got = append(got, p.Name)
	}
	if strings.Join(got, ",") != "field,item,words" {
		t.Errorf("parts = %v, want field,item,words", got)
	}
	if err := registerFieldRuleParts(reg, nil, fieldRuleSync{}); err == nil {
		t.Error("registering twice succeeded")
	}
}

func TestFieldRulePartChecks(t *testing.T) {
	svc, _ := fieldRuleService(t)
	rank := `"rule":{"follows":"echelon","rows":[{"is":"1","gives":"Copper"},{"from":4,"gives":"Starsteel"}]}`
	tests := []struct {
		name string
		kind string
		data string
		want string // normalised data; empty means refused
	}{
		// field
		{"text field, key made from the label", "field", `{"category":"Characters","label":"Home Port"}`,
			`{"category":"Characters","label":"Home Port","key":"home_port","type":"text"}`},
		{"worked-out field keeps its rule", "field", `{"category":"Characters","label":"Guild rank","type":"rule",` + rank + `}`,
			`{"category":"Characters","label":"Guild rank","key":"guild_rank","type":"rule",` + rank + `}`},
		{"link field", "field", `{"category":"Items","label":"Source creature","type":"link","linksTo":"Creatures","section":"Remnant"}`,
			`{"category":"Items","label":"Source creature","key":"source_creature","type":"link","section":"Remnant","linksTo":"Creatures"}`},
		{"select trims options", "field", `{"category":"Items","label":"Kind","type":"select","options":[" Bone ","","Shell"]}`,
			`{"category":"Items","label":"Kind","key":"kind","type":"select","options":["Bone","Shell"]}`},
		{"director-only field", "field", `{"category":"Items","label":"Secret","director":true}`,
			`{"category":"Items","label":"Secret","key":"secret","type":"text","director":true}`},
		{"empty data needs a category", "field", ``, ""},
		{"no label", "field", `{"category":"Items"}`, ""},
		{"unknown setting", "field", `{"category":"Items","label":"A","colour":"red"}`, ""},
		{"unknown type", "field", `{"category":"Items","label":"A","type":"dial"}`, ""},
		{"bad key", "field", `{"category":"Items","label":"A","key":"Big Key"}`, ""},
		{"select without options", "field", `{"category":"Items","label":"A","type":"select"}`, ""},
		{"options on a text field", "field", `{"category":"Items","label":"A","options":["x"]}`, ""},
		{"link without a category", "field", `{"category":"Items","label":"A","type":"link"}`, ""},
		{"linksTo on a text field", "field", `{"category":"Items","label":"A","linksTo":"Creatures"}`, ""},
		{"worked-out field without a rule", "field", `{"category":"Items","label":"A","type":"rule"}`, ""},
		{"rule on a text field", "field", `{"category":"Items","label":"A",` + rank + `}`, ""},
		{"rule row matching nothing", "field", `{"category":"Items","label":"A","type":"rule","rule":{"follows":"x","rows":[{"gives":"y"}]}}`, ""},
		{"rule row with value and range", "field", `{"category":"Items","label":"A","type":"rule","rule":{"follows":"x","rows":[{"is":"1","from":1,"gives":"y"}]}}`, ""},
		{"rule row with unknown setting", "field", `{"category":"Items","label":"A","type":"rule","rule":{"follows":"x","rows":[{"is":"1","gives":"y","when":"z"}]}}`, ""},

		// item
		{"item fields", "item", `{"category":"Items","fields":[{"label":"Source creature","type":"link","linksTo":"Creatures"},{"label":"Remnant kind","type":"select","options":["Bone","Shell"]}]}`,
			`{"category":"Items","fields":[{"label":"Source creature","key":"source_creature","type":"link","linksTo":"Creatures"},{"label":"Remnant kind","key":"remnant_kind","type":"select","options":["Bone","Shell"]}]}`},
		{"item needs fields", "item", `{"category":"Items","fields":[]}`, ""},
		{"item needs a category", "item", `{"fields":[{"label":"A"}]}`, ""},
		{"item field keys are unique", "item", `{"category":"Items","fields":[{"label":"Kind"},{"label":"kind"}]}`, ""},
		{"item field is checked", "item", `{"category":"Items","fields":[{"label":"A","type":"select"}]}`, ""},

		// words
		{"words for a place", "words", `{"placeId":"saltreach","words":[{"word":" Mark ","local":"Grain"},{"word":"Crown","local":"Sack","director":true}]}`,
			`{"placeId":"saltreach","words":[{"word":"Mark","local":"Grain"},{"word":"Crown","local":"Sack","director":true}]}`},
		{"words for an era", "words", `{"era":"The Reign","words":[{"word":"Mark","local":"Bit"}]}`,
			`{"era":"The Reign","words":[{"word":"Mark","local":"Bit"}]}`},
		{"words not yet placed", "words", `{"words":[{"word":"Mark","local":"Bit"}]}`,
			`{"words":[{"word":"Mark","local":"Bit"}]}`},
		{"place and era both", "words", `{"placeId":"saltreach","era":"The Reign","words":[{"word":"Mark","local":"Bit"}]}`, ""},
		{"a page of another campaign", "words", `{"placeId":"elsewhere","words":[{"word":"Mark","local":"Bit"}]}`, ""},
		{"no words", "words", `{"era":"The Reign","words":[]}`, ""},
		{"word without a local name", "words", `{"words":[{"word":"Mark","local":" "}]}`, ""},
		{"same word twice", "words", `{"words":[{"word":"Mark","local":"Bit"},{"word":"mark","local":"Grain"}]}`, ""},
		{"unknown word setting", "words", `{"words":[{"word":"Mark","local":"Bit","dmOnly":true}]}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := homebrew.Part{Key: "p", Kind: tt.kind, Label: "P"}
			if tt.data != "" {
				p.Data = json.RawMessage(tt.data)
			}
			r, err := saveDoc(t, svc, homebrew.Doc{Parts: []homebrew.Part{p}})
			if tt.want == "" {
				if hbCode(err) != 422 {
					t.Fatalf("err = %v, want 422", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := string(r.Doc.Parts[0].Data); got != tt.want {
				t.Errorf("data = %s\nwant   %s", got, tt.want)
			}
		})
	}
}

// TestItemPartNamesTheField checks a refused item field says which one, so
// the Director can find it in a long list.
func TestItemPartNamesTheField(t *testing.T) {
	svc, _ := fieldRuleService(t)
	p := homebrew.Part{Key: "p", Kind: "item", Label: "Remnant", Data: json.RawMessage(
		`{"category":"Items","fields":[{"label":"A"},{"label":"B","type":"select"}]}`)}
	_, err := saveDoc(t, svc, homebrew.Doc{Parts: []homebrew.Part{p}})
	if err == nil || !strings.Contains(err.Error(), "field 2: a select field needs its options") {
		t.Errorf("err = %v, want it to name field 2", err)
	}
}
