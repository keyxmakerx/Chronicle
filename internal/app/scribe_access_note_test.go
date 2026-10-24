package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type fakeScribeNoteCampaigns struct {
	camps   []campaigns.Campaign
	members map[string][]campaigns.CampaignMember
}

func (f *fakeScribeNoteCampaigns) ListAll(_ context.Context, _ campaigns.ListOptions) ([]campaigns.Campaign, int, error) {
	return f.camps, len(f.camps), nil
}

func (f *fakeScribeNoteCampaigns) ListMembers(_ context.Context, id string) ([]campaigns.CampaignMember, error) {
	return f.members[id], nil
}

type sentNote struct {
	to                    []string
	campaign, msg, detail string
	link                  string
}

type fakeScribeNoteBell struct{ sent []sentNote }

func (f *fakeScribeNoteBell) NotifyUsersWithDetail(_ context.Context, to []string, cid, ntype, msg, detail, link string) error {
	if ntype != notifScribeAccessChanged {
		return nil
	}
	f.sent = append(f.sent, sentNote{to, cid, msg, detail, link})
	return nil
}

func TestNoteScribeAccessOnce(t *testing.T) {
	member := func(uid string, role campaigns.Role, name string) campaigns.CampaignMember {
		return campaigns.CampaignMember{UserID: uid, Role: role, DisplayName: name}
	}
	camps := &fakeScribeNoteCampaigns{
		camps: []campaigns.Campaign{
			{ID: "harbor", Name: "Harbor Lights", Settings: `{"dm_grant_ids":["kit"]}`},
			{ID: "codm-only", Name: "Ashfall", Settings: `{"dm_grant_ids":["ada"]}`},
			{ID: "players", Name: "Quiet Road"},
		},
		members: map[string][]campaigns.CampaignMember{
			"harbor": {
				member("dana", campaigns.RoleOwner, "Dana"),
				member("robin", campaigns.RoleScribe, "Robin"),
				member("kit", campaigns.RoleScribe, "Kit"),
				member("theo", campaigns.RolePlayer, "Theo"),
				member("guest", campaigns.RoleScribe, ""),
			},
			"codm-only": {member("lee", campaigns.RoleOwner, "Lee"), member("ada", campaigns.RoleScribe, "Ada")},
			"players":   {member("sam", campaigns.RoleOwner, "Sam"), member("jo", campaigns.RolePlayer, "Jo")},
		},
	}
	st := &fakeExtrasSettings{vals: map[string]string{}}
	bell := &fakeScribeNoteBell{}

	n, err := noteScribeAccessOnce(context.Background(), st, camps, bell)
	if err != nil {
		t.Fatal(err)
	}
	want := []sentNote{{
		to:       []string{"dana"},
		campaign: "harbor",
		msg:      "Scribes in Harbor Lights now see what players see",
		detail:   "Your Scribes: Robin and a guest. To keep one seeing hidden pages, make them a Co-DM on People.",
		link:     "/campaigns/harbor/members",
	}}
	if n != 1 || !reflect.DeepEqual(bell.sent, want) {
		t.Fatalf("sent %d: %+v\nwant %+v", n, bell.sent, want)
	}
	if st.vals[scribeAccessNoteKey] != "1" {
		t.Fatal("run not recorded")
	}

	// A Scribe added after the change is not news: the note goes out once.
	if n, _ := noteScribeAccessOnce(context.Background(), st, camps, bell); n != 0 || len(bell.sent) != 1 {
		t.Fatalf("second run sent %d", n)
	}
}

func TestNameList(t *testing.T) {
	tests := []struct {
		names []string
		want  string
	}{
		{[]string{"Robin"}, "Robin"},
		{[]string{"Robin", "Kit"}, "Robin and Kit"},
		{[]string{"A", "B", "C"}, "A, B and C"},
		{[]string{"A", "B", "C", "D", "E", "F", "G"}, "A, B, C, D, E and 2 more"},
	}
	for _, tt := range tests {
		if got := nameList(tt.names, 5); got != tt.want {
			t.Errorf("nameList(%v) = %q, want %q", tt.names, got, tt.want)
		}
	}
}
