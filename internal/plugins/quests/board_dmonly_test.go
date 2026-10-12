package quests

import (
	"context"
	"testing"
)

// A DM-only category's boards read as missing to anyone below the DM team, and
// its home is left out of the player's list of places with boards.
func TestDMOnlyCategoryBoardsHiddenFromPlayers(t *testing.T) {
	const advType = 77
	fakeDMOnlyTypes[advType] = true
	t.Cleanup(func() { delete(fakeDMOnlyTypes, advType) })

	tests := []struct {
		name string
		v    Viewer
		want int
	}{
		{"dm", dm, 0}, {"scribe", scribe, 0}, {"player", player, 404}, {"guest", guest, 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newBoardEnv()
			e.svc = NewBoardService(e.repo, e.quest, e.ents, fakeTypes{camp: {catType: true, advType: true}}, e.maps, fakeNames{}, nil)
			ctx := context.Background()
			home := TypeHome(advType)
			if _, err := e.svc.CreateBoard(ctx, camp, home, dm, "Prep", WhoAll); err != nil {
				t.Fatal(err)
			}
			if _, err := e.svc.View(ctx, camp, home, tt.v); code(err) != tt.want {
				t.Errorf("View: got %v want %d", err, tt.want)
			}
			homes, err := e.svc.Homes(ctx, camp, tt.v)
			if err != nil {
				t.Fatal(err)
			}
			listed := false
			for _, h := range homes {
				if h.Kind == "category" && h.ID == "77" {
					listed = true
				}
			}
			if listed != (tt.want == 0) {
				t.Errorf("Homes listed the DM-only category = %v, want %v", listed, tt.want == 0)
			}
		})
	}
}
