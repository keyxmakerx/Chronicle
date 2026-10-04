package calendar

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The calendar's own zone reaches the page only for a member looking at a
// real-world calendar; nobody else is shown game nights.
func TestViewerZone(t *testing.T) {
	zone := "America/Chicago"
	real := &Calendar{Mode: ModeRealLife, TracksRealTime: true, RealTimeZone: &zone}
	tests := []struct {
		name string
		role campaigns.Role
		cal  *Calendar
		want string
	}{
		{"member on a real-world calendar", campaigns.RolePlayer, real, zone},
		{"public viewer", campaigns.RoleNone, real, ""},
		{"world calendar", campaigns.RoleOwner, &Calendar{Mode: ModeFantasy}, ""},
		{"real-world calendar with no zone", campaigns.RoleOwner, &Calendar{Mode: ModeRealLife, TracksRealTime: true}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := viewerZone(&campaigns.CampaignContext{MemberRole: tt.role}, tt.cal); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
