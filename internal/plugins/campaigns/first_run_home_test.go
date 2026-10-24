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
	for _, aud := range []string{HomeDefault, HomePlayer, HomeScribe, HomeGuest} {
		if created.HasSavedHome(aud) {
			t.Errorf("%s home should still be the built-in one", aud)
		}
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
