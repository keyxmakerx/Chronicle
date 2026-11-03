package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
)

type carriedDir struct {
	armory.StashDirectory
	ents map[string]*armory.EntityRef
}

func (d carriedDir) GetEntity(_ context.Context, _, id string) (*armory.EntityRef, error) {
	return d.ents[id], nil
}

type carriedAddons bool

func (a carriedAddons) IsEnabledForCampaign(context.Context, string, string) (bool, error) {
	return bool(a), nil
}

// The Armory keeps a carried count only for an owner, with the Armory on,
// between a character and an Armory item; anything else stays a plain edit.
func TestAICarriedAdapterCounts(t *testing.T) {
	dir := carriedDir{ents: map[string]*armory.EntityRef{
		"char": {ID: "char", IsCharacter: true},
		"item": {ID: "item", IsItem: true},
		"page": {ID: "page"},
	}}
	owner := records.Actor{UserID: "o", Role: permissions.RoleOwner}
	tests := []struct {
		name       string
		who        records.Actor
		on         bool
		char, item string
		want       bool
	}{
		{"owner, Armory item", owner, true, "char", "item", true},
		{"a scribe edits by hand", records.Actor{UserID: "s", Role: permissions.RoleScribe}, true, "char", "item", false},
		{"Armory off", owner, false, "char", "item", false},
		{"not an Armory item", owner, true, "char", "page", false},
		{"not a character", owner, true, "page", "item", false},
		{"missing page", owner, true, "char", "gone", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := aiCarriedAdapter{dir: dir, addons: carriedAddons(tc.on)}
			if got := c.Counts(context.Background(), "camp", tc.who, tc.char, tc.item); got != tc.want {
				t.Fatalf("Counts = %v, want %v", got, tc.want)
			}
		})
	}
}

// copiesWords says each remembered copy as the pack does and leaves plain
// copies out.
func TestCopiesWords(t *testing.T) {
	tests := []struct {
		name   string
		copies []armory.Copy
		want   string
	}{
		{"none", nil, ""},
		{"fine and marked, made by", []armory.Copy{{Quantity: 1, Memory: armory.Memory{Quality: armory.QualityFine, Marks: []string{"moonlit"}, How: "made", By: "Orrin", On: "3 Frostmoot"}}},
			"1 Fine · Moonlit (Made by Orrin, 3 Frostmoot)"},
		{"two copies", []armory.Copy{{Quantity: 2, Memory: armory.Memory{Quality: armory.QualityPoor}}, {Quantity: 1, Memory: armory.Memory{Quirk: "hums"}}},
			"2 Poor; 1 (A quirk: hums)"},
		{"plain copy left out", []armory.Copy{{Quantity: 3}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := copiesWords(tt.copies); got != tt.want {
				t.Errorf("copiesWords = %q, want %q", got, tt.want)
			}
		})
	}
}
