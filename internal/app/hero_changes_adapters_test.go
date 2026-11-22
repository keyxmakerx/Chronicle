package app

import (
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
	ch := func(l, f, to string) entities.HeroPickChange { return entities.HeroPickChange{Label: l, From: f, To: to} }
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
