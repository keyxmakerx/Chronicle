package armory

import (
	"encoding/json"
	"testing"
)

func TestShopRoomCounterStarter(t *testing.T) {
	for _, tc := range []struct{ roomType, name string }{
		{"forge", "The Smithy"}, {"apothecary", "The Apothecary"}, {"library", "The Arcane Shop"},
		{"market", "The Night Market"}, {"general", "Plain counter"}, {"jeweler", "Plain counter"}, {"", "Plain counter"},
	} {
		t.Run(tc.roomType, func(t *testing.T) {
			raw := ShopRoomCounterStarter(tc.roomType)
			var c ShopRoomCounter
			if err := json.Unmarshal(raw, &c); err != nil || c.Name != tc.name {
				t.Fatalf("starter = %s (%v), want %q", raw, err, tc.name)
			}
			// Every starter is a theme the validator accepts as it stands.
			if err := c.validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Every starter is a theme the validator accepts as it stands, under a name
// of its own that the by-name lookup finds.
func TestShopRoomCounterStarters_AllValidAndNamed(t *testing.T) {
	var all []ShopRoomCounter
	if err := json.Unmarshal([]byte(shopRoomCounterStartersJSON), &all); err != nil {
		t.Fatal(err)
	}
	if len(all) != 32 {
		t.Fatalf("starters = %d, want 32", len(all))
	}
	seen := map[string]bool{}
	for i := range all {
		c := all[i]
		if seen[c.Name] {
			t.Errorf("two starters are called %q", c.Name)
		}
		seen[c.Name] = true
		if err := c.validate(); err != nil {
			t.Errorf("%s: %v", c.Name, err)
		}
		var got ShopRoomCounter
		if err := json.Unmarshal(ShopRoomCounterStarterNamed(c.Name), &got); err != nil || got.Name != c.Name || got.Skeleton != c.Skeleton {
			t.Errorf("by name %q = %+v (%v)", c.Name, got, err)
		}
	}
	if raw := ShopRoomCounterStarterNamed("The Missing Room"); raw != nil {
		t.Errorf("an unknown name gave %s", raw)
	}
}
