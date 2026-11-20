package app

import (
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// The plan is served as JSON, so the entry key a rule names must survive the
// conversion from the systems list: the hero builder matches rules by it.
func TestHeroChoicesServeKey(t *testing.T) {
	in := []systems.Choice{
		{Key: "fury", Slug: "fury", Name: "Fury", Source: systems.ChoiceSourcePackage},
		{Key: "12", Slug: "mothfolk", Name: "Mothfolk", Source: systems.ChoiceSourceCampaign},
	}
	raw, err := json.Marshal(heroChoices(in))
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"fury", "12"} {
		if got[i]["key"] != want {
			t.Errorf("choice %d: key = %v, want %q", i, got[i]["key"], want)
		}
	}
}
