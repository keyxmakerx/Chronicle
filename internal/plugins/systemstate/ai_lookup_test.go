package systemstate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

// listOnly is a Service that answers ListByKey from a fixed set; the lookup
// must never write, so every other method panics.
type listOnly struct {
	items     map[string][]ListItem // system/key → rows
	truncated bool
	asked     string
}

func (f *listOnly) ListByKey(_ context.Context, _, systemID, key string) ([]ListItem, bool, error) {
	f.asked = systemID + "/" + key
	return f.items[f.asked], f.truncated, nil
}
func (*listOnly) Get(context.Context, string, string, string, string) (State, error) {
	panic("the lookup lists, it never reads one page")
}
func (*listOnly) Put(context.Context, string, string, string, string, *json.RawMessage, *json.RawMessage, string) (State, error) {
	panic("the lookup never writes")
}
func (*listOnly) GetCampaign(context.Context, string, string, string) (State, error) {
	panic("the lookup never reads campaign documents")
}
func (*listOnly) PutCampaign(context.Context, string, string, string, *json.RawMessage, *json.RawMessage, string) (State, error) {
	panic("the lookup never writes")
}

type fixedSystem string

func (s fixedSystem) EnabledSystemID(context.Context, string) string { return string(s) }

func TestAIPageState(t *testing.T) {
	at := time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC)
	svc := &listOnly{items: map[string][]ListItem{
		"drawsteel/negotiation": {
			{EntityID: "e1", EntityName: "Lord Vex", GM: json.RawMessage(`{"attitude":"neutral","interest":2,"patience":3,"motivations":["power"],"pitfalls":["legacy"]}`), Public: json.RawMessage(`{"shown":false,"motivationsFound":[]}`), UpdatedAt: at},
			{EntityID: "e2", EntityName: "Mira", Public: json.RawMessage(`{"shown":true}`), UpdatedAt: at},
		},
	}}
	director := aiplug.Actor{UserID: "o", Role: 3}
	player := aiplug.Actor{UserID: "p", Role: 1}
	rec := func(f map[string]any) aiplug.Record { return aiplug.Record{Kind: "lookup", Fields: f} }
	tests := []struct {
		name    string
		system  fixedSystem
		actor   aiplug.Actor
		rec     aiplug.Record
		want    []string
		notWant []string
		wantErr string
		asked   string
	}{
		{name: "every page with the key, both halves", system: "drawsteel", actor: director, rec: rec(map[string]any{"key": "negotiation"}),
			want: []string{"## negotiation on pages (2)", "### Lord Vex", "Updated 2026-11-20 09:30 UTC.",
				`"motivations":["power"]`, `"pitfalls":["legacy"]`, `{"shown":false,"motivationsFound":[]}`, "### Mira", "Director's half:\n```json\n{}\n```"},
			asked: "drawsteel/negotiation"},
		{name: "one page", system: "drawsteel", actor: director, rec: rec(map[string]any{"key": "negotiation", "page": "mira"}),
			want: []string{"(1)", "### Mira"}, notWant: []string{"Lord Vex"}},
		{name: "a page without it", system: "drawsteel", actor: director, rec: rec(map[string]any{"key": "negotiation", "page": "Nobody"}),
			want: []string{"(0)", `None on "Nobody".`}},
		{name: "a named system", system: "", actor: director, rec: rec(map[string]any{"key": "negotiation", "system": "drawsteel"}),
			want: []string{"(2)"}, asked: "drawsteel/negotiation"},
		{name: "players never get it", system: "drawsteel", actor: player, rec: rec(map[string]any{"key": "negotiation"}), wantErr: "Safe privacy"},
		{name: "a key is needed", system: "drawsteel", actor: director, rec: rec(map[string]any{}), wantErr: "needs `key`"},
		{name: "no system on", system: "", actor: director, rec: rec(map[string]any{"key": "negotiation"}), wantErr: "no game system on"},
		{name: "a malformed key", system: "drawsteel", actor: director, rec: rec(map[string]any{"key": "neg/../x"}), wantErr: "lower-case"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc.asked = ""
			_, text, err := NewAIPageState(svc, tc.system).Answer(context.Background(), "c1", tc.actor, tc.rec)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error %q, got %v", tc.wantErr, err)
				}
				if svc.asked != "" {
					t.Errorf("a refused lookup still read %s", svc.asked)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			for _, w := range tc.want {
				if !strings.Contains(text, w) {
					t.Errorf("missing %q in:\n%s", w, text)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(text, w) {
					t.Errorf("unexpected %q in:\n%s", w, text)
				}
			}
			if tc.asked != "" && svc.asked != tc.asked {
				t.Errorf("asked for %q, want %q", svc.asked, tc.asked)
			}
		})
	}
}

func TestAIPageStateIsOwnerOnly(t *testing.T) {
	in := (&AIPageState{}).LookupInfo()
	if !in.Tool.Owner || in.Tool.Name == "" {
		t.Fatalf("page_state must be an owner-only tool: %+v", in.Tool)
	}
}
