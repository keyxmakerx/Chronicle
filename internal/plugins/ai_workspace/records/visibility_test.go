package records

import (
	"context"
	"testing"
)

// Records take a page's words for who can see as well as their own.
func TestRecord_VisibilityTakesPageWords(t *testing.T) {
	for in, want := range map[string]string{"everyone": "everyone", "dm_only": "dm_only", "public": "everyone", "private": "dm_only", "": "", "party": "party"} {
		if got := rec("pin", ActionCreate, "x", map[string]any{"visibility": in}, "").Visibility(); got != want {
			t.Errorf("Visibility(%q) = %q, want %q", in, got, want)
		}
	}
	k := PinKind{Svc: &fakeMaps{maps: []MapRef{{ID: "m1", CampaignID: camp, Name: "World"}}}, Entities: fakeEntities{}}
	p := k.Plan(context.Background(), camp, player, rec("pin", ActionCreate, "X", map[string]any{"map": "World", "x": 1, "y": 2, "visibility": "private"}, ""))
	if p.Error == "" {
		t.Error("a player added a hidden pin by writing private")
	}
}
