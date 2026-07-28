// app_dashboard_w5c_test.go — C-CAL-DASHBOARD-W5c: the role-aware dashboard
// surface. Proves the player sees only the calendars visible to them (reusing
// the W5a filter through the handler), the owner sees all, and the sort +
// visibility-badge helpers behave.
//
// PIN REFRESH, C-CALV4-BENCH-P4. The surface these drive is now THE BENCH: the
// route is unchanged and so is the guarantee, but the fragment the assertions
// used to read (the card grid, ?grid=1) is now the Bench's SUBORDINATE-row
// section, which by construction holds only the calendars that did not become
// Blocks. Reading it would have made the leak test pass for the wrong reason —
// a player's calendar can be a Block. So the driver renders THE WHOLE PAGE and
// the assertions are unchanged in substance: the dm_only calendar's NAME must
// not appear anywhere in a player's DOM, and must appear in an owner's.
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

// dashboardGridFor drives AppDashboard for a role and returns the rendered
// page — exactly the calendars the viewer gets, wherever on the Bench they
// land. Repo returns one 'everyone' + one 'dm_only' calendar.
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
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/apps/calendar", nil)
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
	// Players get a read-only Bench: no owner-only sort controls, no Permissions
	// trigger, and no New-calendar slot (the create route is Owner-gated, so an
	// affordance a player took would meet a 403 — permission is absence).
	if strings.Contains(body, "data-cal-dashboard-sort") {
		t.Errorf("players must not get the owner sort controls")
	}
	if strings.Contains(body, "data-cal-permissions") {
		t.Errorf("players must not get the Permissions action")
	}
	if strings.Contains(body, "data-bench-newslot") {
		t.Errorf("players must not get the New-calendar slot")
	}
}

func TestAppDashboard_OwnerSeesAllCalendars(t *testing.T) {
	body := dashboardGridFor(t, campaigns.RoleOwner)
	if !strings.Contains(body, "Shared Calendar") || !strings.Contains(body, "GM Secret Calendar") {
		t.Errorf("owner should see ALL calendars (incl. dm_only)")
	}
	// Owner gets the sort controls + the per-calendar Permissions trigger +
	// visibility badges — the W5b affordances the Bench carries on its rows and
	// on each Block's management strip.
	for _, want := range []string{"data-cal-dashboard-sort", "data-cal-permissions", `data-cal-visibility="dm_only"`, `data-cal-visibility="everyone"`} {
		if !strings.Contains(body, want) {
			t.Errorf("owner bench missing %q", want)
		}
	}
}

// The sort control's HTMX fragment still answers on the same route with no new
// route: ?sort=…&grid=1 swaps the Bench's subordinate-row section in place.
func TestAppDashboard_SortFragmentSwapsTheRowSection(t *testing.T) {
	repo := &mockCalendarRepo{
		listByCampaignIDFn: func(_ context.Context, campaignID string) ([]Calendar, error) {
			return []Calendar{
				{ID: "a", CampaignID: campaignID, Name: "Alpha", SortOrder: 1},
				{ID: "b", CampaignID: campaignID, Name: "Beta", SortOrder: 2},
				{ID: "c", CampaignID: campaignID, Name: "Gamma", SortOrder: 3},
			}, nil
		},
		getActiveCalendarIDFn: func(_ context.Context, _, _ string) (string, error) { return "", nil },
	}
	h := NewHandler(NewCalendarService(repo))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/apps/calendar?sort=name&grid=1", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Camp"}, MemberRole: campaigns.RoleOwner,
	})
	c.Set("auth_user_id", "user-1")
	if err := h.AppDashboard(c); err != nil {
		t.Fatalf("AppDashboard: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="cal-dash-grid"`) {
		t.Error("the grid fragment must still be the #cal-dash-grid section the hx-get targets")
	}
	if strings.Contains(body, "data-bench-ribbon") {
		t.Error("the grid fragment is a FRAGMENT — it must not carry the whole page")
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
	sortDashboardCalendars(def, "", nil)
	if def[0].ID != "d" || def[1].ID != "a" || def[2].ID != "b" {
		t.Errorf("default order = %v; want [d a b] (default-first, then sort_order)", ids(def))
	}

	// Name: A→Z regardless of default/sort_order.
	byName := clone()
	sortDashboardCalendars(byName, "name", nil)
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
