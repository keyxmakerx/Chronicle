package armory

import (
	"reflect"
	"testing"
)

// The Items tab always points at the stashes page and the Armory; Crafting
// only when the crafting gate let this viewer through.
func TestSignposts(t *testing.T) {
	tests := []struct {
		name     string
		canCraft bool
		want     []signpost
	}{
		{"crafting off or not for this viewer", false, []signpost{
			{"Party stashes", "/campaigns/c1/armory/stashes"},
			{"Armory", "/campaigns/c1/armory"},
		}},
		{"crafting on", true, []signpost{
			{"Party stashes", "/campaigns/c1/armory/stashes"},
			{"Armory", "/campaigns/c1/armory"},
			{"Crafting", "/campaigns/c1/crafting"},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := signposts("c1", tc.canCraft); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
