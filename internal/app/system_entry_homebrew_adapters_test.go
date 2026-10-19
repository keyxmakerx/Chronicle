package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

type fakeAncestries struct{ director bool }

func (f *fakeAncestries) List(_ context.Context, campaignID, fieldKey string, actor systems.EntryActor) ([]systems.SystemEntry, error) {
	f.director = actor.IsDirector
	if campaignID != "camp" || fieldKey != "ancestry" {
		return nil, nil
	}
	return []systems.SystemEntry{{Slug: "fairy", Name: "Fairy", Visibility: systems.EntryVisibilityCharacter}}, nil
}

func TestAncestryPartChecks(t *testing.T) {
	entries := &fakeAncestries{}
	reg := homebrew.NewRegistry()
	if err := registerAncestryParts(reg, entries); err != nil {
		t.Fatal(err)
	}
	svc := homebrew.NewService(&memRecipes{stored: map[string]homebrew.Recipe{}}, reg, nil, nil)
	tests := []struct {
		name string
		data string
		want string // normalised data; empty means refused
	}{
		{"known slug kept", `{"slug":"fairy"}`, `{"slug":"fairy"}`},
		{"spaces trimmed", `{"slug":" fairy "}`, `{"slug":"fairy"}`},
		{"unknown slug refused", `{"slug":"goblin"}`, ""},
		{"no slug refused", ``, ""},
		{"a copy of the entry refused", `{"slug":"fairy","name":"Fairy"}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := homebrew.Part{Key: "a", Kind: "ancestry", Label: "Fairy", See: homebrew.SeeParty}
			if tt.data != "" {
				p.Data = []byte(tt.data)
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
				t.Errorf("data = %s, want %s", got, tt.want)
			}
			if !entries.director {
				t.Error("looked the entry up as a player, so a penciled ancestry could not be linked")
			}
		})
	}
	if err := registerAncestryParts(reg, entries); err == nil {
		t.Error("registering twice succeeded")
	}
}
