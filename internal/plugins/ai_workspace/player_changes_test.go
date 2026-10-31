package ai_workspace

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakeSheets is one player's characters. PatchSheet refuses any key not in
// the sheet's Free or Picks, the way the real door does, and records writes.
type fakeSheets struct {
	sheets  []PlayerSheet
	applied map[string]map[string]any
}

func (f *fakeSheets) Sheets(_ context.Context, _, userID string) ([]PlayerSheet, error) {
	if userID != "player-1" {
		return nil, nil
	}
	return f.sheets, nil
}

func (f *fakeSheets) PatchSheet(_ context.Context, _, userID, id string, patch map[string]any, _ string, apply bool) error {
	for _, s := range f.sheets {
		if s.ID != id || userID != "player-1" {
			continue
		}
		for k := range patch {
			free := false
			for _, x := range s.Free {
				free = free || x == k
			}
			if _, pick := s.Picks[k]; !free && !pick {
				return apperror.NewForbidden("you can only change your character's identity fields")
			}
		}
		if apply {
			if f.applied == nil {
				f.applied = map[string]map[string]any{}
			}
			f.applied[id] = patch
		}
		return nil
	}
	return apperror.NewNotFound("no such character")
}

func tyneSheets() *fakeSheets {
	return &fakeSheets{sheets: []PlayerSheet{{
		ID: "e-tyne", Name: "Tyne", Type: "Character",
		Values: map[string]string{"pronouns": "she/her", "class": ""},
		Free:   []string{"pronouns"},
		Picks:  map[string][]string{"class": {"Fury", "Tactician"}},
	}}}
}

// Only an update to a character the player claimed, carrying sheet fields
// and nothing else, is kept; everything else is held back with a reason the
// player can read.
func TestPlanPlayerChanges(t *testing.T) {
	tests := []struct {
		name, block, held string
	}{
		{"own sheet", "---\naction: update\nname: Tyne\nreason: She chose a class\nfields:\n  class: Fury\n  pronouns: they/them\n---\n", ""},
		{"name matched without case", "---\naction: update\nname: tyne\nfields:\n  pronouns: they/them\n---\n", ""},
		{"type written out of habit", "---\naction: update\ntype: character\nname: Tyne\nfields:\n  pronouns: they/them\n---\n", ""},
		{"new page", "---\nname: Tyne's Sword\nfields:\n  pronouns: x\n---\n", "can't add or remove pages"},
		{"delete", "---\naction: delete\nname: Tyne\n---\n", "can't add or remove pages"},
		{"someone else's", "---\naction: update\nname: Brann\nfields:\n  pronouns: he/him\n---\n", "isn't a character you claimed"},
		{"page text", "---\naction: update\nname: Tyne\nfields:\n  pronouns: x\n---\nA new backstory.\n", "not the page's text"},
		{"visibility", "---\naction: update\nname: Tyne\nvisibility: public\nfields:\n  pronouns: x\n---\n", "can't set visibility"},
		{"tags", "---\naction: update\nname: Tyne\ntags: [hero]\nfields:\n  pronouns: x\n---\n", "can't set tags"},
		{"a field the sheet door refuses", "---\naction: update\nname: Tyne\nfields:\n  stamina: \"40\"\n---\n", "identity fields"},
		{"a record", "---\nkind: event\nname: Feast\ndate: 1 Deepwinter 1492\n---\n", "is a event"},
		{"a lookup", "---\nkind: lookup\nwhat: pages\n---\n", "lookups aren't answered"},
		{"no fields", "---\naction: update\nname: Tyne\n---\n", "changes no sheet fields"},
		{"nothing new", "---\naction: update\nname: Tyne\nfields:\n  pronouns: she/her\n---\n", "already says this"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sheets := tyneSheets()
			plan, err := planPlayerChanges(context.Background(), sheets, "camp-1", "player-1", tt.block)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan) != 1 {
				t.Fatalf("got %d blocks", len(plan))
			}
			c := plan[0]
			if tt.held == "" {
				if c.Held != "" || c.SheetID != "e-tyne" || c.Page != "Tyne" {
					t.Fatalf("held: %+v", c)
				}
			} else if !strings.Contains(c.Held, tt.held) || c.SheetID != "" || c.Fields != nil {
				t.Fatalf("held = %q, want it to say %q and keep nothing", c.Held, tt.held)
			}
			if sheets.applied != nil {
				t.Fatal("planning wrote to a sheet")
			}
		})
	}
}

// The review shows each field from its current value, and another player
// gets nothing from the same text.
func TestPlanPlayerChanges_LinesAndOtherPlayers(t *testing.T) {
	block := "---\naction: update\nname: Tyne\nfields:\n  pronouns: they/them\n  class: Fury\n---\n"
	plan, err := planPlayerChanges(context.Background(), tyneSheets(), "camp-1", "player-1", block)
	if err != nil {
		t.Fatal(err)
	}
	got := plan[0].Lines
	if len(got) != 2 || got[0] != (PlayerChangeLine{Key: "class", From: "", To: "Fury"}) || got[1] != (PlayerChangeLine{Key: "pronouns", From: "she/her", To: "they/them"}) {
		t.Fatalf("lines = %+v", got)
	}
	if playerCounts(plan) != "1 to change" {
		t.Errorf("counts = %q", playerCounts(plan))
	}
	other, _ := planPlayerChanges(context.Background(), tyneSheets(), "camp-1", "player-2", block)
	if other[0].Held == "" {
		t.Fatal("another player's AI may change Tyne")
	}
	none, _ := planPlayerChanges(context.Background(), nil, "camp-1", "player-1", block)
	if none[0].Held == "" {
		t.Fatal("with no door wired a change was kept")
	}
}

// A field that already holds the value is neither shown nor written.
func TestPlanPlayerChanges_OnlyWhatDiffers(t *testing.T) {
	block := "---\naction: update\nname: Tyne\nfields:\n  pronouns: she/her\n  class: Fury\n---\n"
	plan, err := planPlayerChanges(context.Background(), tyneSheets(), "camp-1", "player-1", block)
	if err != nil {
		t.Fatal(err)
	}
	c := plan[0]
	if c.Held != "" || len(c.Lines) != 1 || c.Lines[0].Key != "class" {
		t.Fatalf("change = %+v", c)
	}
	if _, ok := c.Fields["pronouns"]; ok || c.Fields["class"] != "Fury" {
		t.Fatalf("fields = %v, want only class", c.Fields)
	}
}

// A player's AI is told only about their own characters and the names each
// builder step offers.
func TestPlayerInstructions(t *testing.T) {
	got := playerInstructions("Ashfall", tyneSheets().sheets)
	for _, want := range []string{"### Tyne (Character)", "pronouns: now she/her", "class: (empty). Choose from: Fury, Tactician", "action: update"} {
		if !strings.Contains(got, want) {
			t.Errorf("instructions miss %q:\n%s", want, got)
		}
	}
	if empty := playerInstructions("Ashfall", nil); !strings.Contains(empty, "hasn't claimed a character") {
		t.Errorf("no characters: %s", empty)
	}
}
