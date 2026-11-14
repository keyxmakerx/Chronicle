package campaigns

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

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

type placementFake struct {
	ok    bool
	calls int
	got   []string
	role  int
}

func (p *placementFake) GuestMaySee(_ context.Context, campaignID, blockType, on, userID string, role int) bool {
	p.calls++
	p.got = []string{campaignID, blockType, on, userID}
	p.role = role
	return p.ok
}

// BlockVisibleTo is the one rule behind every placeable fragment.
func TestBlockVisibleTo(t *testing.T) {
	placed := &Campaign{ID: "c1", IsPublic: true}
	full, err := placed.SetRoleDashboardJSON(HomeGuest, &DashboardLayout{Rows: []DashboardRow{{Columns: []DashboardColumn{{Blocks: []DashboardBlock{{Type: BlockNextGame}, {Type: BlockSinceLastHere}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	placed.DashboardLayout = full
	plain := &Campaign{ID: "c1", IsPublic: true}
	guest := func(c *Campaign) *CampaignContext { return &CampaignContext{Campaign: c} }
	member := &CampaignContext{Campaign: plain, IsMember: true, MemberRole: RolePlayer}

	tests := []struct {
		name      string
		cc        *CampaignContext
		checker   *placementFake
		block, on string
		want      bool
		wantAsked bool
	}{
		{"member passes anywhere", member, &placementFake{}, BlockNextGame, "", true, false},
		{"member passes on a page", member, &placementFake{}, BlockNextGame, "p1", true, false},
		{"member passes since_last_here", member, &placementFake{}, BlockSinceLastHere, "p1", true, false},
		{"guest on the Guests home", guest(placed), &placementFake{}, BlockNextGame, "", true, false},
		{"guest on a page the checker allows", guest(plain), &placementFake{ok: true}, BlockNextGame, "p1", true, true},
		{"guest on a page the checker refuses", guest(plain), &placementFake{}, BlockNextGame, "p1", false, true},
		{"guest with no placement", guest(plain), &placementFake{ok: true}, BlockNextGame, "", false, false},
		{"guest on the home never reaches the checker for another block", guest(placed), &placementFake{}, BlockOpenQuests, "", false, false},
		{"since_last_here: guest on the Guests home", guest(placed), &placementFake{ok: true}, BlockSinceLastHere, "", false, false},
		{"since_last_here: guest on a page", guest(plain), &placementFake{ok: true}, BlockSinceLastHere, "p1", false, false},
		{"missing context", nil, &placementFake{ok: true}, BlockNextGame, "p1", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BlockVisibleTo(context.Background(), tt.cc, tt.checker, tt.block, tt.on, "u9")
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			if (tt.checker.calls > 0) != tt.wantAsked {
				t.Errorf("checker asked = %v, want %v", tt.checker.calls > 0, tt.wantAsked)
			}
			if tt.wantAsked {
				if want := []string{"c1", tt.block, tt.on, "u9"}; strings.Join(tt.checker.got, "|") != strings.Join(want, "|") || tt.checker.role != 0 {
					t.Errorf("checker asked %v role %d, want %v role 0", tt.checker.got, tt.checker.role, want)
				}
			}
		})
	}
	// No checker wired: only the Guests home counts.
	if BlockVisibleTo(context.Background(), guest(plain), nil, BlockNextGame, "p1", "") {
		t.Error("with no checker a page placement must not count")
	}
}

func TestBlockFragmentURL(t *testing.T) {
	tests := []struct {
		campaign, block, on, want string
		ok                        bool
	}{
		{"c1", BlockNextGame, "", "/campaigns/c1/next-game", true},
		{"c1", BlockNextGame, "p 1&x", "/campaigns/c1/next-game?on=p+1%26x", true},
		{"a/b", BlockWorldClocks, "p1", "/campaigns/a%2Fb/world-clocks?on=p1", true},
		{"c1", "party", "p1", "", false},
	}
	for _, tt := range tests {
		got, ok := BlockFragmentURL(tt.campaign, tt.block, tt.on)
		if got != tt.want || ok != tt.ok {
			t.Errorf("BlockFragmentURL(%q,%q,%q) = %q,%v want %q,%v", tt.campaign, tt.block, tt.on, got, ok, tt.want, tt.ok)
		}
	}
	// The home draws the same fragment, without "on".
	for _, bt := range PlaceableBlockTypes() {
		if _, ok := BlockFragmentURL("c1", bt, ""); !ok {
			t.Errorf("%s has no fragment address", bt)
		}
	}
}

// OwnerCardVisible is the check the activity and sync cards make themselves:
// the owner only, and not while looking as a player. A co-Director is not
// shown them, because the routes behind them answer the owner alone.
func TestOwnerCardVisible(t *testing.T) {
	camp := &Campaign{ID: "c1"}
	tests := []struct {
		name string
		cc   *CampaignContext
		ctx  context.Context
		want bool
	}{
		{"owner", &CampaignContext{Campaign: camp, MemberRole: RoleOwner, IsMember: true}, context.Background(), true},
		{"owner looking as a player", &CampaignContext{Campaign: camp, MemberRole: RoleOwner, IsMember: true}, layouts.SetViewingAsPlayer(context.Background(), true), false},
		{"co-Director", &CampaignContext{Campaign: camp, MemberRole: RolePlayer, IsMember: true, IsDmGranted: true}, context.Background(), false},
		{"scribe", &CampaignContext{Campaign: camp, MemberRole: RoleScribe, IsMember: true}, context.Background(), false},
		{"player", &CampaignContext{Campaign: camp, MemberRole: RolePlayer, IsMember: true}, context.Background(), false},
		{"guest", &CampaignContext{Campaign: camp, MemberRole: RoleNone}, context.Background(), false},
		{"no context", nil, context.Background(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OwnerCardVisible(tt.ctx, tt.cc); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// The home draws the activity and sync cards through the same check, marked
// for the Director team, and an owner looking as a player is shown neither.
func TestOwnerCardsOnTheHomeAreMarkedAndHiddenFromAPlayersView(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: RoleOwner, IsMember: true}
	for _, bt := range []string{"activity_feed", "sync_status"} {
		out := renderCardWithCtx(t, context.Background(), DashboardBlockSwitch(cc, DashboardBlock{Type: bt}, nil))
		if !strings.Contains(out, "data-dm-only") {
			t.Errorf("%s: not marked for the Director team:\n%s", bt, out)
		}
		out = renderCardWithCtx(t, layouts.SetViewingAsPlayer(context.Background(), true), DashboardBlockSwitch(cc, DashboardBlock{Type: bt}, nil))
		if strings.TrimSpace(out) != "" {
			t.Errorf("%s: shown to an owner looking as a player:\n%s", bt, out)
		}
	}
}
