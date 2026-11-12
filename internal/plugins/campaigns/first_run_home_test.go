package campaigns

import (
	"bytes"
	"context"
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
	if first.ID != setupStepsBlockID || first.Type != BlockTextBlock {
		t.Fatalf("first block = %+v, want the setup steps text block", first)
	}
	for _, aud := range []string{HomePlayer, HomeScribe, HomeGuest} {
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
		{HomePlayer, "start_here,recent_pages", "at_a_glance,category_grid"},
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

// The steps render through the Text Block as links into this campaign.
func TestSetupSteps_RenderAsLinks(t *testing.T) {
	block := starterDirectorHome("camp-9").Rows[1].Columns[0].Blocks[0]
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
	block := starterDirectorHome("camp-9").Rows[1].Columns[0].Blocks[0]
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
