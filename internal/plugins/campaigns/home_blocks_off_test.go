package campaigns

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// A block whose feature is off must leave no fragment shell behind: the shell
// is an hx-get that answers 404, which stays a grey bar and raises a toast.
// Only the owner is told why a placed block is missing. A guest is drawn the
// same blocks as a member where the owner placed them on the Guests home, so
// the guest rows hold for those.
func TestDashboardBlockSwitch_FeatureOff(t *testing.T) {
	viewers := map[string]*CampaignContext{
		"player": {Campaign: &Campaign{ID: "c1"}, MemberRole: RolePlayer, IsMember: true},
		"owner":  {Campaign: &Campaign{ID: "c1"}, MemberRole: RoleOwner, IsMember: true},
		"guest":  {Campaign: &Campaign{ID: "c1"}},
	}
	type row struct {
		block, feature string
		viewers        []string
	}
	rows := []row{
		{BlockWhenWePlay, "The calendar", []string{"player", "owner"}},
		{BlockLastTime, "The calendar", []string{"player", "owner"}},
		{BlockGettingStarted, "Game nights", []string{"player", "owner"}},
		{BlockWhenWeCouldPlay, "Game nights", []string{"player", "owner", "guest"}},
		{BlockNextGame, "The calendar", []string{"player", "owner", "guest"}},
		{BlockTodayInWorld, "The calendar", []string{"player", "owner", "guest"}},
		{BlockTimelinePreview, "Timelines", []string{"player", "owner"}},
		{BlockTimelineFull, "Timelines", []string{"player", "owner"}},
		{BlockMapFull, "Maps", []string{"player", "owner"}},
	}
	for _, r := range rows {
		for _, who := range r.viewers {
			t.Run(r.block+"/"+who, func(t *testing.T) {
				ctx := layouts.SetHomeBlocksOff(context.Background(), map[string]string{r.block: r.feature})
				out := renderCardWithCtx(t, ctx, DashboardBlockSwitch(viewers[who], DashboardBlock{ID: "b", Type: r.block}, nil))
				for _, bad := range []string{"hx-get", "animate-pulse", "aria-busy", "data-widget", "Loading"} {
					if strings.Contains(out, bad) {
						t.Errorf("draws %q with its feature off:\n%s", bad, out)
					}
				}
				wantNote := who == "owner"
				if got := strings.Contains(out, "data-block-off"); got != wantNote {
					t.Errorf("owner note = %v, want %v:\n%s", got, wantNote, out)
				}
				if wantNote && !strings.Contains(out, r.feature) {
					t.Errorf("note does not name %q:\n%s", r.feature, out)
				}
			})
		}
	}
}

// With nothing recorded as off, a block still draws its fragment shell, so
// the check removes a block only when told its feature is off.
func TestDashboardBlockSwitch_FeatureOn(t *testing.T) {
	player := &CampaignContext{Campaign: &Campaign{ID: "c1"}, MemberRole: RolePlayer, IsMember: true}
	out := renderCardWithCtx(t, context.Background(), DashboardBlockSwitch(player, DashboardBlock{ID: "b", Type: BlockLastTime}, nil))
	if !strings.Contains(out, `hx-get="/campaigns/c1/last-time"`) {
		t.Errorf("last time lost its fragment with its feature on:\n%s", out)
	}
	off := layouts.SetHomeBlocksOff(context.Background(), map[string]string{BlockNextGame: "The calendar"})
	out = renderCardWithCtx(t, off, DashboardBlockSwitch(player, DashboardBlock{ID: "b", Type: BlockLastTime}, nil))
	if !strings.Contains(out, "last-time") {
		t.Errorf("a block of another feature was hidden:\n%s", out)
	}
}

// A block placed on an ordinary page goes through PageBlockFragment, which
// must obey the same rule.
func TestPageBlockFragment_FeatureOff(t *testing.T) {
	ctx := layouts.SetHomeBlocksOff(context.Background(), map[string]string{BlockNextGame: "The calendar"})
	if out := renderCardWithCtx(t, ctx, PageBlockFragment(BlockNextGame, "c1", "")); strings.TrimSpace(out) != "" {
		t.Errorf("next game drawn with its feature off: %s", out)
	}
	if out := renderCardWithCtx(t, ctx, PageBlockFragment(BlockOpenQuests, "c1", "")); !strings.Contains(out, "open-quests") {
		t.Errorf("open quests lost: %s", out)
	}
}
