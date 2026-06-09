// app_dashboard_w5c_test.go — C-CAL-DASHBOARD-W5c: the role-aware dashboard
// surface. Proves the player sees only the calendars visible to them (reusing
// the W5a filter through the handler), the owner sees all, and the card-grid
// sort + visibility-badge helpers behave.
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

// dashboardGridFor drives AppDashboard for a role and returns the rendered card
// grid (the HTMX grid fragment — layout-free, exactly the calendars the viewer
// gets). Repo returns one 'everyone' + one 'dm_only' calendar.
func dashboardGridFor(t *testing.T, role campaigns.Role) string {
	t.Helper()
	repo := &mockCalendarRepo{
		listByCampaignIDFn: func(_ context.Context, campaignID string) ([]Calendar, error) {
			return []Calendar{
				{ID: "open", CampaignID: campaignID, Name: "Shared Calendar", Visibility: "everyone"},
				{ID: "secret", CampaignID: campaignID, Name: "GM Secret Calendar", Visibility: "dm_only"},
			}, nil
		},
		getActiveCalendarIDFn: func(_ context.Context, _, _ string) (string, error) { return "", nil },
	}
	h := NewHandler(NewCalendarService(repo))

	e := echo.New()
	// grid=1 + HX-Request → the handler returns just the grid fragment.
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/apps/calendar?grid=1", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Camp"}, MemberRole: role,
	})
	c.Set("auth_user_id", "user-1")

	if err := h.AppDashboard(c); err != nil {
		t.Fatalf("AppDashboard(role=%d): %v", role, err)
	}
	return rec.Body.String()
}

func TestAppDashboard_PlayerSeesOnlyVisibleCalendars(t *testing.T) {
	body := dashboardGridFor(t, campaigns.RolePlayer)
	if !strings.Contains(body, "Shared Calendar") {
		t.Errorf("player should see the 'everyone' calendar")
	}
	if strings.Contains(body, "GM Secret Calendar") {
		t.Errorf("player must NOT see the dm_only calendar (W5a filter through the handler)")
	}
	// Players get read-only cards: no owner-only sort controls or Permissions stub.
	if strings.Contains(body, "data-cal-dashboard-sort") {
		t.Errorf("players must not get the owner sort controls")
	}
	if strings.Contains(body, "data-cal-permissions-stub") {
		t.Errorf("players must not get the Permissions action")
	}
}

func TestAppDashboard_OwnerSeesAllCalendars(t *testing.T) {
	body := dashboardGridFor(t, campaigns.RoleOwner)
	if !strings.Contains(body, "Shared Calendar") || !strings.Contains(body, "GM Secret Calendar") {
		t.Errorf("owner should see ALL calendars (incl. dm_only)")
	}
	// Owner gets the sort controls + the per-card Permissions stub + visibility badges.
	for _, want := range []string{"data-cal-dashboard-sort", "data-cal-permissions", `data-cal-visibility="dm_only"`, `data-cal-visibility="everyone"`} {
		if !strings.Contains(body, want) {
			t.Errorf("owner grid missing %q", want)
		}
	}
}

func TestSortDashboardCalendars(t *testing.T) {
	base := []Calendar{
		{ID: "b", Name: "Beta", SortOrder: 2},
		{ID: "a", Name: "Alpha", SortOrder: 1},
		{ID: "d", Name: "Delta", SortOrder: 0, IsDefault: true},
	}
	clone := func() []Calendar { return append([]Calendar(nil), base...) }

	// Default: is_default first, then sort_order.
	def := clone()
	sortDashboardCalendars(def, "")
	if def[0].ID != "d" || def[1].ID != "a" || def[2].ID != "b" {
		t.Errorf("default order = %v; want [d a b] (default-first, then sort_order)", ids(def))
	}

	// Name: A→Z regardless of default/sort_order.
	byName := clone()
	sortDashboardCalendars(byName, "name")
	if byName[0].Name != "Alpha" || byName[1].Name != "Beta" || byName[2].Name != "Delta" {
		t.Errorf("name order = %v; want Alpha,Beta,Delta", ids(byName))
	}
}

func TestNormalizeCalendarSort(t *testing.T) {
	for _, k := range []string{"name", "created", "updated", ""} {
		if got := normalizeCalendarSort(k); got != k {
			t.Errorf("normalizeCalendarSort(%q) = %q; want %q", k, got, k)
		}
	}
	if got := normalizeCalendarSort("bogus; drop table"); got != "" {
		t.Errorf("unknown sort should clamp to default; got %q", got)
	}
}

func TestCalVisibilityKind(t *testing.T) {
	cases := []struct {
		name string
		cal  Calendar
		want string
	}{
		{"everyone default", Calendar{Visibility: "everyone"}, "everyone"},
		{"dm_only", Calendar{Visibility: "dm_only"}, "dm_only"},
		{"everyone + rules = custom", Calendar{Visibility: "everyone", VisibilityRules: strptr(`{"denied_users":["u1"]}`)}, "custom"},
		{"empty rules object is not custom", Calendar{Visibility: "everyone", VisibilityRules: strptr(`{}`)}, "everyone"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := calVisibilityKind(tc.cal); got != tc.want {
				t.Errorf("calVisibilityKind = %q; want %q", got, tc.want)
			}
		})
	}
}
