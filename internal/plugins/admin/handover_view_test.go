package admin

import (
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestLastSeenText(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		ago  time.Duration
		want string
	}{
		{time.Hour, "seen today"},
		{30 * time.Hour, "seen yesterday"},
		{10 * 24 * time.Hour, "seen 10 days ago"},
		{200 * 24 * time.Hour, "seen 6 months ago"},
	}
	for _, tt := range tests {
		if got := lastSeenText(now.Add(-tt.ago), now); got != tt.want {
			t.Errorf("lastSeenText(-%v) = %q, want %q", tt.ago, got, tt.want)
		}
	}
}

func TestHandoverCandidatesLeaveOutOwnerAndGuests(t *testing.T) {
	got := handoverCandidates([]campaigns.CampaignMember{
		{UserID: "owner"}, {UserID: "pat"}, {UserID: "gil", IsGuest: true}, {UserID: "sam"},
	}, "owner")
	if len(got) != 2 || got[0].UserID != "pat" || got[1].UserID != "sam" {
		t.Errorf("candidates = %v, want pat and sam", got)
	}
}

func TestHandoverActionOnlyOnceOpen(t *testing.T) {
	c := campaigns.Campaign{ID: "c1"}
	if acts := handoverAction(c, campaigns.HandoverStatus{AfterMonths: 6}); len(acts) != 0 {
		t.Errorf("closed hand-over offered %v", acts)
	}
	acts := handoverAction(c, campaigns.HandoverStatus{AfterMonths: 6, Open: true})
	if len(acts) != 1 || acts[0].URL != "/admin/campaigns/c1/hand-over" || acts[0].Method != "get" {
		t.Errorf("open hand-over = %v", acts)
	}
}
