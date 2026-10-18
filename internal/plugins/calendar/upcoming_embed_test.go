package calendar

import (
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestPreviewUpcomingEvents_NoCalendarForViewer pins who is told no calendar
// is set up: only the owner. Anyone else may have every calendar hidden from
// them, so they get an empty 200 and the card holding the fragment drops.
func TestPreviewUpcomingEvents_NoCalendarForViewer(t *testing.T) {
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner, "u-scribe": campaigns.RoleScribe, "u-player": campaigns.RolePlayer}
	tests := []struct {
		name    string
		userID  string
		wantSet bool
	}{
		{"owner is told to set one up", "u-owner", true},
		{"scribe gets an empty body", "u-scribe", false},
		{"player gets an empty body", "u-player", false},
		{"public visitor gets an empty body", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, svc := newAccessTestRouter(true, true, roles)
			svc.noDefault = true
			rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/upcoming", tc.userID)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			body := rec.Body.String()
			if tc.wantSet && !strings.Contains(body, "No calendar has been set up yet.") {
				t.Errorf("owner body = %q, want the set-up text", body)
			}
			if !tc.wantSet && strings.TrimSpace(body) != "" {
				t.Errorf("body = %q, want empty", body)
			}
		})
	}
}
