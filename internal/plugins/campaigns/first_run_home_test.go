package campaigns

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"strings"
	"testing"
)

// A new campaign starts with new pages hidden from players and the setup
// steps on the DM team's home, and only there.
func TestCreate_StartsWithSetupStepsAndHiddenPages(t *testing.T) {
	var created *Campaign
	repo := &mockCampaignRepo{createFn: func(_ context.Context, c *Campaign) error { created = c; return nil }}
	svc := newTestCampaignServiceFull(repo, &mockUserFinder{}, nil, &mockSeeder{})
	if _, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{Name: "Test"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := created.ParseSettings().DefaultVisibility; got != DefaultVisibilityDMOnly {
		t.Errorf("DefaultVisibility = %q, want %q", got, DefaultVisibilityDMOnly)
	}
	home, own := created.HomeLayout(HomeDirector)
	if !own {
		t.Fatal("the DM team's home should be its own layout")
	}
	first := home.Rows[1].Columns[0].Blocks[0]
	if first.ID != setupStepsBlockID || first.Type != BlockSetupSteps {
		t.Fatalf("first block = %+v, want the setup steps block", first)
	}
	for _, aud := range []string{HomeScribe, HomeGuest} {
		if created.HasSavedHome(aud) {
			t.Errorf("%s home should not have a layout of its own", aud)
		}
	}
	if !created.HasSavedHome(HomeDefault) {
		t.Error("members should start on the saved front page")
	}
}

// New campaigns start on the signed front page: a main column with Start here
// and the latest pages beside a side column led by At a glance. Only the DM
// team's version holds the owner-only activity feed and links, so no member
// is placed a block that can't draw for them.
func TestCreate_StartsOnTheFrontPage(t *testing.T) {
	var created *Campaign
	repo := &mockCampaignRepo{createFn: func(_ context.Context, c *Campaign) error { created = c; return nil }}
	svc := newTestCampaignServiceFull(repo, &mockUserFinder{}, nil, &mockSeeder{})
	if _, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{Name: "Test"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	types := func(c DashboardColumn) string {
		var out []string
		for _, b := range c.Blocks {
			out = append(out, b.Type)
		}
		return strings.Join(out, ",")
	}
	tests := []struct {
		audience, main, side string
	}{
		{HomeScribe, "start_here,recent_pages", "at_a_glance,category_grid"},
		{HomeDirector, "start_here,recent_pages,activity_feed", "at_a_glance,category_grid,quick_actions"},
	}
	for _, tt := range tests {
		t.Run(tt.audience, func(t *testing.T) {
			home, _ := created.HomeLayout(tt.audience)
			if err := validateDashboardLayout(context.Background(), created.ID, home); err != nil {
				t.Fatalf("layout does not validate: %v", err)
			}
			if home.Rows[0].Columns[0].Blocks[0].Type != BlockWelcomeBanner {
				t.Errorf("the campaign's name should lead the page")
			}
			front := home.Rows[len(home.Rows)-1]
			if len(front.Columns) != 2 || front.Columns[0].Width != 8 || front.Columns[1].Width != 4 {
				t.Fatalf("front row = %+v, want an 8 and 4 column pair", front.Columns)
			}
			if got := types(front.Columns[0]); got != tt.main {
				t.Errorf("main column = %s, want %s", got, tt.main)
			}
			if got := types(front.Columns[1]); got != tt.side {
				t.Errorf("side column = %s, want %s", got, tt.side)
			}
		})
	}
	if guest, own := created.HomeLayout(HomeGuest); own || guest.Rows[len(guest.Rows)-1].Columns[0].Blocks[0].Type == BlockStartHere {
		t.Error("guests keep the built-in guest home")
	}
}

// New campaigns start Players on the signed players' home: the page to start
// on, last time and the latest pages beside the next game, the party, when and
// where we play and the house rules. The layout must validate, and holds no
// owner-only block, since every block in it draws for a player.
func TestCreate_StartsPlayersOnThePlayersHome(t *testing.T) {
	var created *Campaign
	repo := &mockCampaignRepo{createFn: func(_ context.Context, c *Campaign) error { created = c; return nil }}
	svc := newTestCampaignServiceFull(repo, &mockUserFinder{}, nil, &mockSeeder{})
	if _, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{Name: "Test"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	home, own := created.HomeLayout(HomePlayer)
	if !own {
		t.Fatal("players should start on a home of their own")
	}
	if err := validateDashboardLayout(context.Background(), created.ID, home); err != nil {
		t.Fatalf("layout does not validate: %v", err)
	}
	row := home.Rows[len(home.Rows)-1]
	if len(row.Columns) != 2 || row.Columns[0].Width != 7 || row.Columns[1].Width != 5 {
		t.Fatalf("players row = %+v, want a 7 and 5 column pair", row.Columns)
	}
	var main, side []string
	for _, b := range row.Columns[0].Blocks {
		main = append(main, b.Type)
	}
	for _, b := range row.Columns[1].Blocks {
		side = append(side, b.Type)
	}
	if got := strings.Join(main, ","); got != "start_here,last_time,recent_pages" {
		t.Errorf("main column = %s", got)
	}
	if got := strings.Join(side, ","); got != "coming_up,party,when_we_play,where_we_play,house_rules" {
		t.Errorf("side column = %s", got)
	}
	for _, r := range home.Rows {
		for _, c := range r.Columns {
			for _, b := range c.Blocks {
				if b.Type == BlockActivityFeed || b.Type == BlockQuickActions {
					t.Errorf("%s is the DM team's, not a player's", b.Type)
				}
			}
		}
	}
}

// legacySetupBlock is the Text Block of setup steps that campaigns made before
// the setup line hold in their saved home. They keep it as it is.
func legacySetupBlock(campaignID string) DashboardBlock {
	return DashboardBlock{ID: setupStepsBlockID, Type: BlockTextBlock, Config: map[string]any{"content": legacySetupStepsHTML(campaignID)}}
}

// The older Text Block of steps renders as links into this campaign.
func TestSetupSteps_LegacyRenderAsLinks(t *testing.T) {
	block := legacySetupBlock("camp-9")
	var buf bytes.Buffer
	if err := dashTextBlock(&CampaignContext{Campaign: &Campaign{ID: "camp-9"}, MemberRole: RoleOwner}, block.Config).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`href="/campaigns/camp-9/extensions"`, `href="/campaigns/camp-9/members"`,
		`href="/campaigns/camp-9/customize/look"`, `href="/campaigns/camp-9/entities/new"`,
		`href="/campaigns/camp-9/customize/home/director"`, "Get your table ready",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered steps missing %s\n%s", want, out)
		}
	}
}

// The setup note is the owner's checklist; a co-DM gets their own short note
// whose links they can open, and a player gets neither.
func TestSetupNote_ByRole(t *testing.T) {
	block := legacySetupBlock("camp-9")
	ownerLinks := []string{"/extensions", "/members", "/customize/look", "/customize/home/director"}
	tests := []struct {
		name     string
		cc       *CampaignContext
		wantLink []string
		noLink   []string
		wantText []string
		nothing  bool
	}{
		{
			name:     "owner gets the four owner steps",
			cc:       &CampaignContext{Campaign: &Campaign{ID: "camp-9"}, MemberRole: RoleOwner},
			wantLink: append([]string{"/entities/new"}, ownerLinks...),
			wantText: []string{"Get your table ready"},
		},
		{
			name:     "a scribe with DM access gets their own note and none of the owner's pages",
			cc:       &CampaignContext{Campaign: &Campaign{ID: "camp-9"}, MemberRole: RoleScribe, IsDmGranted: true},
			wantLink: []string{"/campaigns/camp-9/entities/new"},
			noLink:   []string{"/extensions", "/members", "/customize"},
			wantText: []string{"amber edge", "eye button", "The owner sets the game, the look and who&#39;s invited"},
		},
		{
			name:    "a player gets neither note",
			cc:      &CampaignContext{Campaign: &Campaign{ID: "camp-9"}, MemberRole: RolePlayer},
			nothing: true,
		},
		{
			name:    "a scribe without DM access gets neither note",
			cc:      &CampaignContext{Campaign: &Campaign{ID: "camp-9"}, MemberRole: RoleScribe},
			nothing: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := DashboardBlockSwitch(tt.cc, block, nil).Render(context.Background(), &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if tt.nothing {
				if strings.TrimSpace(out) != "" {
					t.Errorf("expected nothing, got:\n%s", out)
				}
				return
			}
			for _, l := range tt.wantLink {
				if !strings.Contains(out, "/campaigns/camp-9"+strings.TrimPrefix(l, "/campaigns/camp-9")) {
					t.Errorf("missing link %s\n%s", l, out)
				}
			}
			for _, l := range tt.noLink {
				if strings.Contains(out, l) {
					t.Errorf("a co-DM is shown %s, which refuses them:\n%s", l, out)
				}
			}
			for _, w := range tt.wantText {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q\n%s", w, out)
				}
			}
		})
	}
}

// The setup line is the owner's: the owner's home loads it, a co-Director gets
// their own note, and everyone else gets nothing.
func TestSetupLine_ByRole(t *testing.T) {
	block := starterDirectorHome("camp-9").Rows[1].Columns[0].Blocks[0]
	tests := []struct {
		name     string
		cc       *CampaignContext
		want     []string
		nothing  bool
		noOwners bool
	}{
		{"owner loads the line", &CampaignContext{Campaign: &Campaign{ID: "camp-9"}, MemberRole: RoleOwner}, []string{`hx-get="/campaigns/camp-9/setup-steps"`}, false, false},
		{"co-director gets their note", &CampaignContext{Campaign: &Campaign{ID: "camp-9"}, MemberRole: RoleScribe, IsDmGranted: true}, []string{"amber edge"}, false, true},
		{"player gets nothing", &CampaignContext{Campaign: &Campaign{ID: "camp-9"}, MemberRole: RolePlayer}, nil, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := DashboardBlockSwitch(tt.cc, block, nil).Render(context.Background(), &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if tt.nothing && strings.TrimSpace(out) != "" {
				t.Fatalf("expected nothing, got:\n%s", out)
			}
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q\n%s", w, out)
				}
			}
			if tt.noOwners && strings.Contains(out, "/setup-steps") {
				t.Errorf("a co-Director is shown the owner's line:\n%s", out)
			}
		})
	}
}

// An empty Text Block is a placeholder for the owner only; players get no card.
func TestTextBlock_EmptyShowsOnlyToOwner(t *testing.T) {
	for _, tt := range []struct {
		name string
		role Role
		want bool
	}{
		{"owner", RoleOwner, true},
		{"player", RolePlayer, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cc := &CampaignContext{Campaign: &Campaign{ID: "c1"}, MemberRole: tt.role}
			var buf bytes.Buffer
			if err := dashTextBlock(cc, map[string]any{"content": ""}).Render(context.Background(), &buf); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(buf.String(), "Empty text block"); got != tt.want {
				t.Errorf("placeholder shown = %v, want %v:\n%s", got, tt.want, buf.String())
			}
		})
	}
}

// A co-DM keeps text the owner rewrote without the owner-only links.
func TestSetupNote_CoDMKeepsTheOwnersRewrite(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "c"}, MemberRole: RoleScribe, IsDmGranted: true}
	block := DashboardBlock{ID: setupStepsBlockID, Type: BlockTextBlock, Config: map[string]any{"content": "<p>House rules below.</p>"}}
	got, show := setupNoteFor(context.Background(), cc, block)
	if !show || got.Config["content"] != "<p>House rules below.</p>" {
		t.Errorf("rewritten note changed: show=%v %v", show, got.Config)
	}
}

// legacySetupStepsHTML is the setup block's text: four steps, each a link to the
// page that does it, and where to remove the block. The wording avoids the
// game master's title because no game system is chosen yet.
func legacySetupStepsHTML(campaignID string) string {
	base := "/campaigns/" + html.EscapeString(campaignID)
	return fmt.Sprintf(`<h3>Get your table ready</h3>
<p>Only the people running this campaign see this note.</p>
<ol>
<li><a href="%[1]s/extensions">Choose your game</a> and turn on the parts you'll use, like the calendar and maps.</li>
<li><a href="%[1]s/members">Invite your players</a>. One link works for new and returning players.</li>
<li><a href="%[1]s/customize/look">Pick a look</a> for your campaign.</li>
<li><a href="%[1]s/entities/new">Write your first page</a>: a place, a villain, a hook. New pages start hidden from players until you show them.</li>
</ol>
<p>Done? Remove this note in <a href="%[1]s/customize/home/director">Customize → Home pages</a>.</p>`, base)
}

