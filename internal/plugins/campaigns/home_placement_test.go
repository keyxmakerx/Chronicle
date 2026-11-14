package campaigns

import "testing"

func TestHomeHasBlock(t *testing.T) {
	col := &DashboardLayout{Rows: []DashboardRow{{Columns: []DashboardColumn{{Blocks: []DashboardBlock{{Type: "party"}, {Type: BlockNextGame}}}}}}}
	none := &DashboardLayout{Rows: []DashboardRow{{Columns: []DashboardColumn{{Blocks: []DashboardBlock{{Type: "party"}}}}}}}
	tests := []struct {
		name string
		l    *DashboardLayout
		typ  string
		want bool
	}{
		{"in a column", col, BlockNextGame, true},
		{"another block in the column", col, BlockHouseRules, false},
		{"not placed", none, BlockNextGame, false},
		{"nil layout", nil, BlockNextGame, false},
	}
	for _, tt := range tests {
		if got := HomeHasBlock(tt.l, tt.typ); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

// Only a layout the owner saved for the Guests home counts; the built-in
// starter home places none of the small blocks, so a visitor sees nothing
// until the owner chooses.
func TestGuestHomeHasBlock(t *testing.T) {
	camp := &Campaign{ID: "c1", IsPublic: true}
	if GuestHomeHasBlock(camp, BlockNextGame) {
		t.Error("the built-in guest home must not place the block")
	}
	full, err := camp.SetRoleDashboardJSON(HomeGuest, &DashboardLayout{Rows: []DashboardRow{{Columns: []DashboardColumn{{Blocks: []DashboardBlock{{Type: BlockNextGame}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	camp.DashboardLayout = full
	if !GuestHomeHasBlock(camp, BlockNextGame) || GuestHomeHasBlock(camp, "party") {
		t.Error("only the placed block counts")
	}
	// A members' home that places the block is not the guests' home.
	members := &Campaign{ID: "c2", IsPublic: true}
	full, err = members.SetRoleDashboardJSON(HomePlayer, &DashboardLayout{Rows: []DashboardRow{{Columns: []DashboardColumn{{Blocks: []DashboardBlock{{Type: BlockNextGame}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	members.DashboardLayout = full
	if GuestHomeHasBlock(members, BlockNextGame) {
		t.Error("a block on the Players home must not reach guests")
	}
}
