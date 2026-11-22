package app

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func TestHeroChangeRecipients(t *testing.T) {
	c := &campaigns.Campaign{ID: "c1"}
	members := []campaigns.CampaignMember{
		{UserID: "own", Role: campaigns.RoleOwner, DisplayName: "Dana"},
		{UserID: "pl", Role: campaigns.RolePlayer, DisplayName: " Ana "},
		{UserID: "pl2", Role: campaigns.RolePlayer, DisplayName: "Bo"},
		{UserID: "scr", Role: campaigns.RoleScribe, DisplayName: "Sam"},
	}
	got, player := heroChangeRecipients(c, members, "pl")
	if strings.Join(got, ",") != "own" || player != "Ana" {
		t.Fatalf("recipients %v, player %q", got, player)
	}
	// The owner changing their own hero does not ring their own bell.
	got, _ = heroChangeRecipients(c, members, "own")
	if len(got) != 0 {
		t.Fatalf("the writer was told of their own change: %v", got)
	}
}

func TestHeroChangeDetail(t *testing.T) {
	ch := func(l, f, to string) entities.HeroPickChange {
		return entities.HeroPickChange{Label: l, From: f, To: to}
	}
	cases := []struct {
		in   []entities.HeroPickChange
		want string
	}{
		{[]entities.HeroPickChange{ch("Class", "Fury", "Tactician")}, "Class: Fury → Tactician"},
		{[]entities.HeroPickChange{ch("Kit", "Panther", "")}, "Kit: Panther → nothing"},
		{[]entities.HeroPickChange{ch("A", "1", "2"), ch("B", "1", "2"), ch("C", "1", "2"), ch("D", "1", "2"), ch("E", "1", "2")},
			"A: 1 → 2; B: 1 → 2; C: 1 → 2; and 2 more"},
	}
	for _, tc := range cases {
		if got := heroChangeDetail(tc.in); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

// fakeHeroCamps is one campaign with an owner, a Co-DM (a Scribe with the
// DM grant), a plain Scribe and two players.
type fakeHeroCamps struct{ heroChangeCampaigns }

func (fakeHeroCamps) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	return &campaigns.Campaign{ID: id, Settings: `{"dm_grant_ids":["codm"]}`}, nil
}

func (fakeHeroCamps) ListMembers(context.Context, string) ([]campaigns.CampaignMember, error) {
	return []campaigns.CampaignMember{
		{UserID: "own", Role: campaigns.RoleOwner, DisplayName: "Dana"},
		{UserID: "pl", Role: campaigns.RolePlayer, DisplayName: "Ana"},
		{UserID: "pl2", Role: campaigns.RolePlayer, DisplayName: "Bo"},
		{UserID: "codm", Role: campaigns.RoleScribe, DisplayName: "Cora"},
		{UserID: "scr", Role: campaigns.RoleScribe, DisplayName: "Sam"},
	}, nil
}

// fakeBell keeps the one bell it was asked to send.
type fakeBell struct {
	to                           []string
	ntype, message, detail, link string
}

func (b *fakeBell) NotifyUsersWithDetail(_ context.Context, to []string, _, ntype, message, detail, link string) error {
	b.to, b.ntype, b.message, b.detail, b.link = to, ntype, message, detail, link
	return nil
}

// The level bell goes to the owner and the Co-DM, not the player, another
// player or a plain Scribe, and says only who,
// which hero and the new level, whatever else the page holds.
func TestHeroChangeBell_LevelRaised(t *testing.T) {
	bell := &fakeBell{}
	b := heroChangeBell{camps: fakeHeroCamps{}, bell: bell}
	e := &entities.Entity{ID: "e1", CampaignID: "c1", Name: "Tess",
		FieldsData: map[string]any{"level": float64(3), "gm_notes": "secret villain"}}
	if err := b.LevelRaised(context.Background(), e, "pl", 3); err != nil {
		t.Fatal(err)
	}
	if strings.Join(bell.to, ",") != "own,codm" {
		t.Errorf("sent to %v, want the owner and the Co-DM only", bell.to)
	}
	if bell.ntype != notifHeroLevelRaised {
		t.Errorf("type %q", bell.ntype)
	}
	if bell.message != "Ana raised Tess to level 3" || bell.detail != "" {
		t.Errorf("message %q, detail %q", bell.message, bell.detail)
	}
	if bell.link != "/campaigns/c1/entities/e1" {
		t.Errorf("link %q", bell.link)
	}
}
