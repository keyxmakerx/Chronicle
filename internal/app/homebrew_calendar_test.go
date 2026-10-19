package app

import (
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

func calendarHomebrewService(t *testing.T) homebrew.Service {
	t.Helper()
	reg := homebrew.NewRegistry()
	if err := registerCalendarHomebrew(reg); err != nil {
		t.Fatal(err)
	}
	if err := registerCalendarHomebrew(reg); err == nil {
		t.Error("registering twice succeeded")
	}
	return homebrew.NewService(&memRecipes{stored: map[string]homebrew.Recipe{}}, reg, nil, nil)
}

func TestCalendarHomebrewRules(t *testing.T) {
	svc := calendarHomebrewService(t)
	const effect = `{"name":"Watched","who":"party","see":"party","lasts":"until_change"}`
	rule := func(event, eventArgs, actionArgs string) homebrew.Rule {
		return homebrew.Rule{
			When: homebrew.When{Event: event, Args: json.RawMessage(eventArgs)},
			Then: []homebrew.Do{{Action: "party.effect", Args: json.RawMessage(actionArgs)}},
		}
	}
	moon := func(args string) homebrew.Rule { return rule("moon.changed", args, effect) }
	roll := func(args string) homebrew.Rule { return rule("calendar.roll", args, effect) }
	fx := func(args string) homebrew.Rule { return rule("moon.changed", `{"moon":"Umbra"}`, args) }
	tests := []struct {
		name     string
		rule     homebrew.Rule
		wantWhen string // normalised args; empty means refused
		wantThen string
	}{
		{"moon any change", moon(`{"moon":" Umbra "}`), `{"moon":"Umbra"}`, effect},
		{"moon to a state", moon(`{"moon":"Umbra","state":"Dark"}`), `{"moon":"Umbra","state":"dark"}`, effect},
		{"moon unknown state", moon(`{"moon":"Umbra","state":"gibbous"}`), "", ""},
		{"moon needs a name", moon(`{"state":"full"}`), "", ""},
		{"moon unknown field", moon(`{"moon":"Umbra","phase":0.5}`), "", ""},
		{"moon name with a newline", moon(`{"moon":"Um\nbra"}`), "", ""},

		{"roll any result", roll(`{"event":"Night market"}`), `{"event":"Night market"}`, effect},
		{"roll one result", roll(`{"event":"Night market","result":"Raided"}`), `{"event":"Night market","result":"Raided"}`, effect},
		{"roll needs an event", roll(`{"result":"Raided"}`), "", ""},

		{"effect on a tag for days", fx(`{"name":"Moonsick","text":"Bane on Presence tests","who":"tag","tag":"Remnant","see":"director","lasts":"duration","duration":3}`),
			`{"moon":"Umbra"}`, `{"name":"Moonsick","text":"Bane on Presence tests","who":"tag","tag":"Remnant","see":"director","lasts":"duration","duration":3}`},
		{"party drops a stray tag", fx(`{"name":"W","who":"party","tag":"x","see":"party","lasts":"until_change","duration":4}`),
			`{"moon":"Umbra"}`, `{"name":"W","who":"party","see":"party","lasts":"until_change"}`},
		{"effect needs a name", fx(`{"who":"party","see":"party","lasts":"until_change"}`), "", ""},
		{"tag needs a tag", fx(`{"name":"W","who":"tag","see":"party","lasts":"until_change"}`), "", ""},
		{"unknown who", fx(`{"name":"W","who":"everyone","see":"party","lasts":"until_change"}`), "", ""},
		{"unknown see", fx(`{"name":"W","who":"party","see":"name","lasts":"until_change"}`), "", ""},
		{"duration needs days", fx(`{"name":"W","who":"party","see":"party","lasts":"duration"}`), "", ""},
		{"duration too long", fx(`{"name":"W","who":"party","see":"party","lasts":"duration","duration":3651}`), "", ""},
		{"unknown lasts", fx(`{"name":"W","who":"party","see":"party","lasts":"forever"}`), "", ""},
		{"text too long", fx(`{"name":"W","text":"` + longText(301) + `","who":"party","see":"party","lasts":"until_change"}`), "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := saveDoc(t, svc, homebrew.Doc{Rules: []homebrew.Rule{tt.rule}})
			if tt.wantWhen == "" {
				if hbCode(err) != 422 {
					t.Fatalf("err = %v, want 422", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := r.Doc.Rules[0]
			if string(got.When.Args) != tt.wantWhen {
				t.Errorf("when args = %s, want %s", got.When.Args, tt.wantWhen)
			}
			if string(got.Then[0].Args) != tt.wantThen {
				t.Errorf("then args = %s, want %s", got.Then[0].Args, tt.wantThen)
			}
		})
	}

	t.Run("a calendar event takes no part", func(t *testing.T) {
		r := moon(`{"moon":"Umbra"}`)
		r.When.Part = "need"
		doc := homebrew.Doc{
			Parts: []homebrew.Part{{Key: "need", Kind: "tracker", Label: "Need", See: homebrew.SeeParty}},
			Rules: []homebrew.Rule{r},
		}
		if _, err := saveDoc(t, svc, doc); hbCode(err) != 422 {
			t.Fatalf("err = %v, want 422", err)
		}
	})
}

func longText(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
