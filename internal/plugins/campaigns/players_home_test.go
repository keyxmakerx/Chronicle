package campaigns

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

func TestWhereWePlayLink(t *testing.T) {
	tests := []struct {
		name, in, text, href string
	}{
		{"bare host gets https", "foundry.example.com", "foundry.example.com", "https://foundry.example.com"},
		{"https kept", "https://foundry.example.com/join", "https://foundry.example.com/join", "https://foundry.example.com/join"},
		{"http kept", "http://192.168.1.4:30000", "http://192.168.1.4:30000", "http://192.168.1.4:30000"},
		{"host and port", "foundry.example.com:30000", "foundry.example.com:30000", "https://foundry.example.com:30000"},
		{"localhost", "localhost:30000", "localhost:30000", "https://localhost:30000"},
		{"javascript is text", "javascript:alert(1)", "javascript:alert(1)", ""},
		{"javascript with a dot is text", "javascript:alert(document.domain)", "javascript:alert(document.domain)", ""},
		{"data is text", "data:text/html,<b>x</b>", "data:text/html,<b>x</b>", ""},
		{"other scheme is text", "ftp://files.example.com", "ftp://files.example.com", ""},
		{"spaces are text", "my foundry server", "my foundry server", ""},
		{"empty", "   ", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WhereWePlayLink(tt.in)
			if got.Text != tt.text || got.Href != tt.href {
				t.Errorf("WhereWePlayLink(%q) = %+v, want text %q href %q", tt.in, got, tt.text, tt.href)
			}
		})
	}
	if long := WhereWePlayLink(strings.Repeat("a", 1000) + ".com"); len(long.Text) != maxWhereWePlayLen {
		t.Errorf("an over-long address is cut to %d, got %d", maxWhereWePlayLen, len(long.Text))
	}
}

func renderBlock(t *testing.T, ctx context.Context, cc *CampaignContext, block DashboardBlock) string {
	t.Helper()
	var sb strings.Builder
	if err := DashboardBlockSwitch(cc, block, nil).Render(ctx, &sb); err != nil {
		t.Fatalf("render %s: %v", block.Type, err)
	}
	return sb.String()
}

// Where we play draws what the owner typed, links only a web address, and
// escapes the rest; with nothing typed it is the owner's dashed slot and
// nothing at all for a player.
func TestDashWhereWePlay(t *testing.T) {
	cc := func(role Role, member bool) *CampaignContext {
		return &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: role, IsMember: member}
	}
	cfg := func(url, voice string) map[string]any {
		return map[string]any{"foundry_url": url, "voice": voice}
	}
	block := func(c map[string]any) DashboardBlock {
		return DashboardBlock{ID: "b", Type: BlockWhereWePlay, Config: c}
	}

	html := renderBlock(t, context.Background(), cc(RolePlayer, true), block(cfg("foundry.example.com", "Discord, the Ember Coast")))
	for _, want := range []string{`href="https://foundry.example.com"`, "Open the game", "Discord, the Ember Coast", `rel="noopener noreferrer"`} {
		if !strings.Contains(html, want) {
			t.Errorf("player's block missing %q:\n%s", want, html)
		}
	}

	html = renderBlock(t, context.Background(), cc(RolePlayer, true), block(cfg("javascript:alert(1)", "<script>x</script>")))
	if strings.Contains(html, "href=\"javascript") || strings.Contains(html, "<script>") {
		t.Errorf("an unsafe address or text reached the page as live markup:\n%s", html)
	}
	if strings.Contains(html, "Open the game") {
		t.Error("an address that is not a web address must not get an Open button")
	}

	if html = renderBlock(t, context.Background(), cc(RolePlayer, true), block(cfg("", ""))); strings.TrimSpace(html) != "" {
		t.Errorf("a player is shown nothing when nothing is typed, got:\n%s", html)
	}
	if html = renderBlock(t, context.Background(), cc(RoleOwner, true), block(cfg("", ""))); !strings.Contains(html, "type your Foundry address") {
		t.Errorf("the owner should get the slot saying where to type it:\n%s", html)
	}
	previewing := layouts.SetViewingAsPlayer(context.Background(), true)
	if html = renderBlock(t, previewing, cc(RoleOwner, true), block(cfg("", ""))); strings.TrimSpace(html) != "" {
		t.Errorf("the owner viewing as a player is shown what a player is shown, got:\n%s", html)
	}
}

// The members-only blocks are not placed for a visitor, whose request the
// block's route would refuse.
func TestDashFragmentBlocks_MembersOnly(t *testing.T) {
	for _, typ := range []string{BlockWhenWePlay, BlockHouseRules, BlockLastTime} {
		visitor := &CampaignContext{Campaign: &Campaign{ID: "camp-1", IsPublic: true}}
		if html := renderBlock(t, context.Background(), visitor, DashboardBlock{ID: "b", Type: typ}); strings.TrimSpace(html) != "" {
			t.Errorf("%s drew for a visitor:\n%s", typ, html)
		}
		member := &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: RolePlayer, IsMember: true}
		if html := renderBlock(t, context.Background(), member, DashboardBlock{ID: "b", Type: typ}); !strings.Contains(html, "hx-get") {
			t.Errorf("%s should load its fragment for a member:\n%s", typ, html)
		}
	}
	// The party is public-capable: what a visitor may see is decided by the
	// fragment, as Start here is.
	visitor := &CampaignContext{Campaign: &Campaign{ID: "camp-1", IsPublic: true}}
	if html := renderBlock(t, context.Background(), visitor, DashboardBlock{ID: "b", Type: BlockParty}); !strings.Contains(html, "/campaigns/camp-1/party") {
		t.Errorf("the party block should load its fragment:\n%s", html)
	}
}

// The eye takes the DM team to the players' home, with the strip saying so;
// everyone else, and the DM team not previewing, see their own home.
func TestShowPage_EyeShowsPlayersHome(t *testing.T) {
	campaign := &Campaign{ID: "camp-1", Name: "Test"}
	home, err := campaign.SetRoleDashboardJSON(HomePlayer, &DashboardLayout{Rows: []DashboardRow{{ID: "r", Columns: []DashboardColumn{{
		ID: "c", Width: 12, Blocks: []DashboardBlock{{ID: "b", Type: BlockTextBlock, Config: map[string]any{"content": "<p>PLAYERS-HOME-MARKER</p>"}}},
	}}}}})
	if err != nil {
		t.Fatal(err)
	}
	campaign.DashboardLayout = home
	if home, err = campaign.SetRoleDashboardJSON(HomeDirector, &DashboardLayout{Rows: []DashboardRow{{ID: "r", Columns: []DashboardColumn{{
		ID: "c", Width: 12, Blocks: []DashboardBlock{{ID: "b", Type: BlockTextBlock, Config: map[string]any{"content": "<p>DIRECTOR-HOME-MARKER</p>"}}},
	}}}}}); err != nil {
		t.Fatal(err)
	}
	campaign.DashboardLayout = home

	render := func(cc *CampaignContext, ctx context.Context) string {
		var sb strings.Builder
		if err := CampaignShowPage(cc, nil, nil, "tok").Render(ctx, &sb); err != nil {
			t.Fatalf("render: %v", err)
		}
		return sb.String()
	}
	owner := &CampaignContext{Campaign: campaign, MemberRole: RoleOwner, IsMember: true}
	player := &CampaignContext{Campaign: campaign, MemberRole: RolePlayer, IsMember: true}
	preview := layouts.SetViewingAsPlayer(context.Background(), true)

	got := render(owner, preview)
	if !strings.Contains(got, "PLAYERS-HOME-MARKER") || strings.Contains(got, "DIRECTOR-HOME-MARKER") {
		t.Errorf("the eye should show the players' home, not the director's:\n%s", got)
	}
	for _, want := range []string{"You are seeing the players&#39; home", "/campaigns/camp-1/customize/home/player", "Back to your own home", "toggle-view-mode"} {
		if !strings.Contains(got, want) && !strings.Contains(got, strings.ReplaceAll(want, "&#39;", "'")) {
			t.Errorf("strip missing %q", want)
		}
	}

	if got = render(owner, context.Background()); !strings.Contains(got, "DIRECTOR-HOME-MARKER") || strings.Contains(got, "players' home, as a player") {
		t.Errorf("without the eye the owner keeps their own home and no strip")
	}
	if got = render(player, context.Background()); !strings.Contains(got, "PLAYERS-HOME-MARKER") || strings.Contains(got, "Back to your own home") {
		t.Errorf("a player sees the players' home with no strip")
	}
	// A co-DM previewing is not the owner: the strip has no Customizer link.
	coDM := &CampaignContext{Campaign: campaign, MemberRole: RoleScribe, IsMember: true, IsDmGranted: true}
	if got = render(coDM, preview); strings.Contains(got, "customize/home/player") {
		t.Errorf("only the owner is offered the Customizer")
	}
}
