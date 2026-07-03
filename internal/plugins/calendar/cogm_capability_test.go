// cogm_capability_test.go — C-CAL-COGM-CAPABILITY (Phase 3) calendar side:
// a co-DM (DM-grantee) can author dm_only events and a plain Scribe is
// downgraded. (The V1 eventModal UI-gate test was retired with the V1 view
// layer in C-CAL-CLOSEOUT PR C; the V2 drawer's gating is covered by the
// calendar_v2_* tests.)
package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// createEventVisibility drives CreateEventAPI with the given role/grant and a
// dm_only request, returning the visibility the service actually persisted.
func createEventVisibility(t *testing.T, role campaigns.Role, dmGranted bool) string {
	t.Helper()
	var got string
	repo := &mockCalendarRepo{
		getByCampaignIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Harptos"}, nil
		},
		createEventFn: func(_ context.Context, evt *Event) error { got = evt.Visibility; return nil },
	}
	h := NewHandler(NewCalendarService(repo))

	e := echo.New()
	body := `{"name":"Eclipse","year":1,"month":1,"day":1,"visibility":"dm_only"}`
	req := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role, IsDmGranted: dmGranted,
	})
	c.Set("auth_user_id", "user-1")
	if err := h.CreateEventAPI(c); err != nil {
		t.Fatalf("CreateEventAPI: %v", err)
	}
	return got
}

func TestCoDM_CanAuthorDmOnly(t *testing.T) {
	// A DM-granted Scribe (co-DM) keeps dm_only.
	if got := createEventVisibility(t, campaigns.RoleScribe, true); got != "dm_only" {
		t.Errorf("co-DM dm_only should persist, got %q", got)
	}
	// An Owner keeps dm_only.
	if got := createEventVisibility(t, campaigns.RoleOwner, false); got != "dm_only" {
		t.Errorf("owner dm_only should persist, got %q", got)
	}
	// A plain Scribe is downgraded to everyone.
	if got := createEventVisibility(t, campaigns.RoleScribe, false); got != "everyone" {
		t.Errorf("plain scribe dm_only should downgrade to everyone, got %q", got)
	}
	// A plain Player is downgraded too.
	if got := createEventVisibility(t, campaigns.RolePlayer, false); got != "everyone" {
		t.Errorf("plain player dm_only should downgrade to everyone, got %q", got)
	}
}
