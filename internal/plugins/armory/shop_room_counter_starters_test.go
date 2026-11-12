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
