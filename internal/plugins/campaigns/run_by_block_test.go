package campaigns

import (
	"testing"
	"time"
)

// Run by names the owner first, then co-Directors, and nobody else.
func TestRunByPeople(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	camp := &Campaign{ID: "c1"}
	camp.Settings = `{"dm_grant_ids":["u-co"]}`
	members := []CampaignMember{
		{UserID: "u-co", Role: RolePlayer, DisplayName: "Priya", JoinedAt: t0.Add(time.Hour)},
		{UserID: "u-p", Role: RolePlayer, DisplayName: "Sam", JoinedAt: t0.Add(2 * time.Hour)},
		{UserID: "u-o", Role: RoleOwner, DisplayName: "Key", JoinedAt: t0.Add(3 * time.Hour)},
		{UserID: "u-anon", Role: RoleOwner, DisplayName: ""},
	}
	got := runByPeople(camp, members, "DM")
	want := []runByPerson{{"Key", "Owner"}, {"Priya", "Co-DM"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %v, want %v", i, got[i], want[i])
		}
	}
}
