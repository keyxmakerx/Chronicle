// cogm_capability_test.go — C-CAL-COGM-CAPABILITY (Phase 3) calendar side:
// a co-DM (DM-grantee) can author dm_only events, a plain Scribe is
// downgraded, and the "DM Only" UI option only renders when the viewer can
// actually author it (the UI-lie fix).
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

// TestEventModal_DmOnlyOptionGated: the "DM Only" visibility option only
// renders for someone who can author it (the Scribe UI-lie fix).
func TestEventModal_DmOnlyOptionGated(t *testing.T) {
	cal := &Calendar{ID: "cal-1", Name: "Harptos"}

	render := func(canAuthor bool) string {
		var sb strings.Builder
		data := CalendarViewData{Calendar: cal, CanAuthorDmOnly: canAuthor}
		if err := eventModal(data).Render(context.Background(), &sb); err != nil {
			t.Fatalf("render eventModal: %v", err)
		}
		return sb.String()
	}

	if html := render(true); !strings.Contains(html, `value="dm_only"`) {
		t.Errorf("co-DM/owner should see the DM Only option")
	}
	if html := render(false); strings.Contains(html, `value="dm_only"`) {
		t.Errorf("a non-author (Scribe) must NOT see the DM Only option (the UI-lie)")
	}
}
