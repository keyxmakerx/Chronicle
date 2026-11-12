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
