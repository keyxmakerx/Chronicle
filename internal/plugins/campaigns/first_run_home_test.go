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
	if err := dashTextBlock(block.Config).Render(context.Background(), &buf); err != nil {
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
