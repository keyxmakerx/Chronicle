// subresource_v2_test.go covers V2 Wave 1 PR 2 (C-CAL-V2-
// SUBRESOURCE-CARDS-A) sub-resource card-grid handler + helpers.
// Handler tests use the same httptest harness as audit_log_test.go;
// helper tests pin the per-resource card-data projection.

package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- Helper-projection tests ---

func TestMonthsToCards_SubtitleComposition(t *testing.T) {
	months := []Month{
		{Name: "Mirtul", Days: 30},
		{Name: "Shieldmeet", Days: 1, IsIntercalary: true},
		{Name: "Tarsakh", Days: 30, LeapYearDays: 1},
	}
	cards := monthsToCards(months)
	if len(cards) != 3 {
		t.Fatalf("expected 3 cards; got %d", len(cards))
	}
	if cards[0].Subtitle != "30 days" {
		t.Errorf("Mirtul subtitle = %q; want '30 days'", cards[0].Subtitle)
	}
	if !strings.Contains(cards[1].Subtitle, "intercalary") || !strings.Contains(cards[1].Subtitle, "1 day") {
		t.Errorf("Shieldmeet subtitle = %q; want intercalary + '1 day'", cards[1].Subtitle)
	}
	if !strings.Contains(cards[2].Subtitle, "+1 leap") {
		t.Errorf("Tarsakh subtitle = %q; want '+1 leap'", cards[2].Subtitle)
	}
}

func TestWeekdaysToCards_RestDayAccent(t *testing.T) {
	weekdays := []Weekday{
		{Name: "Sul"},
		{Name: "Mol", IsRestDay: true},
	}
	cards := weekdaysToCards(weekdays)
	if cards[0].IsAccent {
		t.Error("Sul should not be accent")
	}
	if !cards[1].IsAccent {
		t.Error("Mol (rest day) should be accent")
	}
	if cards[1].Subtitle != "rest day" {
		t.Errorf("Mol subtitle = %q; want 'rest day'", cards[1].Subtitle)
	}
}

func TestMoonsToCards_ColorPropagates(t *testing.T) {
	moons := []Moon{{Name: "Selune", CycleDays: 29.5, Color: "#abcdef"}}
	cards := moonsToCards(moons)
	if cards[0].Color != "#abcdef" {
		t.Errorf("expected color propagation; got %q", cards[0].Color)
	}
	if !strings.Contains(cards[0].Subtitle, "cycle 29d") {
		t.Errorf("moon subtitle = %q; want 'cycle 29d'", cards[0].Subtitle)
	}
}

func TestSeasonsToCards_RangeSubtitle(t *testing.T) {
	seasons := []Season{{Name: "Summer", StartMonth: 6, StartDay: 1, EndMonth: 8, EndDay: 31, Color: "#ffcc00"}}
	cards := seasonsToCards(seasons)
	if cards[0].Color != "#ffcc00" {
		t.Errorf("expected color propagation; got %q", cards[0].Color)
	}
	if !strings.Contains(cards[0].Subtitle, "month 6") || !strings.Contains(cards[0].Subtitle, "month 8") {
		t.Errorf("season subtitle = %q; want range with month 6 + 8", cards[0].Subtitle)
	}
}

func TestSubresourcePayloadJSON_RoundTrip(t *testing.T) {
	data := SubresourceViewData{
		Kind:    SubresourceMonths,
		Months:  []Month{{Name: "Jan", Days: 31}, {Name: "Feb", Days: 28}},
	}
	got := subresourcePayloadJSON(data)
	var roundTrip []Month
	if err := json.Unmarshal([]byte(got), &roundTrip); err != nil {
		t.Fatalf("payload JSON not parseable: %v (got %q)", err, got)
	}
	if len(roundTrip) != 2 || roundTrip[0].Name != "Jan" {
		t.Errorf("round-trip mismatch: %+v", roundTrip)
	}
}

func TestSubresourcePayloadJSON_UnknownKindReturnsEmptyArray(t *testing.T) {
	data := SubresourceViewData{Kind: SubresourceKind("nonsense")}
	got := subresourcePayloadJSON(data)
	if got != "[]" {
		t.Errorf("expected '[]' for unknown kind; got %q", got)
	}
}

func TestSubresourcePUTPath_ReusesV1Endpoint(t *testing.T) {
	got := subresourcePUTPath("camp-1", "cal-2", SubresourceMonths)
	want := "/campaigns/camp-1/calendars/cal-2/months"
	if got != want {
		t.Errorf("PUT path = %q; want %q", got, want)
	}
}

func TestSubresourceTitle_AllKinds(t *testing.T) {
	cases := map[SubresourceKind]string{
		SubresourceMonths:   "Months",
		SubresourceWeekdays: "Weekdays",
		SubresourceMoons:    "Moons",
		SubresourceSeasons:  "Seasons",
	}
	for k, want := range cases {
		if got := subresourceTitle(k); got != want {
			t.Errorf("subresourceTitle(%q) = %q; want %q", k, got, want)
		}
	}
}

func TestPluralizeDays(t *testing.T) {
	if pluralizeDays(1) != "1 day" {
		t.Errorf("pluralizeDays(1) = %q; want '1 day'", pluralizeDays(1))
	}
	if pluralizeDays(7) != "7 days" {
		t.Errorf("pluralizeDays(7) = %q; want '7 days'", pluralizeDays(7))
	}
}

func TestSubresourceEmptyExplainer_ResourceSpecific(t *testing.T) {
	// Sanity: each kind returns non-empty + distinct copy. Operator
	// guidance varies meaningfully per resource.
	seen := map[string]bool{}
	for _, k := range []SubresourceKind{SubresourceMonths, SubresourceWeekdays, SubresourceMoons, SubresourceSeasons} {
		s := subresourceEmptyExplainer(k)
		if s == "" {
			t.Errorf("empty explainer for %q", k)
		}
		if seen[s] {
			t.Errorf("duplicate explainer copy: %q", s)
		}
		seen[s] = true
	}
}

// --- Handler tests ---

func TestShowV2SubresourceSettings_MonthsPath(t *testing.T) {
	months := []Month{{Name: "Jan", Days: 31}, {Name: "Feb", Days: 28}}
	cal := &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Test", Months: months}
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id == "cal-1" {
				return cal, nil
			}
			return nil, nil
		},
		getMonthsFn: func(_ context.Context, _ string) ([]Month, error) { return months, nil },
	}
	h := NewHandler(NewCalendarService(repo))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "calId", "resource")
	c.SetParamValues("camp-1", "cal-1", "months")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner,
	})
	c.Set("auth_user_id", "user-1")

	if err := h.ShowV2SubresourceSettings(c); err != nil {
		t.Fatalf("ShowV2SubresourceSettings: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Jan") || !strings.Contains(body, "Feb") {
		t.Errorf("response missing month names; got %d bytes", len(body))
	}
	// Confirm the payload JSON was inlined for the JS widget.
	if !strings.Contains(body, "data-subresource-kind=\"months\"") {
		t.Errorf("response missing subresource-kind attribute; got %d bytes", len(body))
	}
}

func TestShowV2SubresourceSettings_UnknownResource404s(t *testing.T) {
	cal := &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Test"}
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
	}
	h := NewHandler(NewCalendarService(repo))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "calId", "resource")
	c.SetParamValues("camp-1", "cal-1", "nonsense")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner,
	})
	c.Set("auth_user_id", "user-1")

	err := h.ShowV2SubresourceSettings(c)
	if err == nil {
		t.Fatal("expected NotFound for unknown resource")
	}
}

func TestShowV2SubresourceSettings_EmptyStateRenders(t *testing.T) {
	cal := &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Test", Moons: nil}
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
	}
	h := NewHandler(NewCalendarService(repo))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "calId", "resource")
	c.SetParamValues("camp-1", "cal-1", "moons")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner,
	})
	c.Set("auth_user_id", "user-1")

	if err := h.ShowV2SubresourceSettings(c); err != nil {
		t.Fatalf("ShowV2SubresourceSettings: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "No Moons") {
		t.Errorf("expected empty-state heading 'No Moons'; got body of %d bytes", len(body))
	}
}

func TestShowV2SubresourceSettings_PlayerNoAddCard(t *testing.T) {
	weekdays := []Weekday{{Name: "Sul"}}
	cal := &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Test", Weekdays: weekdays}
	repo := &mockCalendarRepo{
		getByIDFn:    func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
		getWeekdaysFn: func(_ context.Context, _ string) ([]Weekday, error) { return weekdays, nil },
	}
	h := NewHandler(NewCalendarService(repo))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "calId", "resource")
	c.SetParamValues("camp-1", "cal-1", "weekdays")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer,
	})
	c.Set("auth_user_id", "user-2")

	if err := h.ShowV2SubresourceSettings(c); err != nil {
		t.Fatalf("ShowV2SubresourceSettings: %v", err)
	}
	body := rec.Body.String()
	// Player sees cards but no "Add weekday" affordance and no drawer.
	if !strings.Contains(body, "Sul") {
		t.Errorf("expected card render for player; got %d bytes", len(body))
	}
	if strings.Contains(body, "data-add-card") {
		t.Error("player should not see add-card affordance")
	}
	if strings.Contains(body, "subresource-drawer") {
		t.Error("player should not see drawer DOM")
	}
}

// TestShowV2SubresourceSettings_HTMXRespondsWithFragment — HTMX
// requests get the grid fragment, not the full page.
func TestShowV2SubresourceSettings_HTMXRespondsWithFragment(t *testing.T) {
	months := []Month{{Name: "Jan", Days: 31}}
	cal := &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Test", Months: months}
	repo := &mockCalendarRepo{
		getByIDFn:   func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
		getMonthsFn: func(_ context.Context, _ string) ([]Month, error) { return months, nil },
	}
	h := NewHandler(NewCalendarService(repo))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "calId", "resource")
	c.SetParamValues("camp-1", "cal-1", "months")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner,
	})
	c.Set("auth_user_id", "user-1")

	if err := h.ShowV2SubresourceSettings(c); err != nil {
		t.Fatalf("ShowV2SubresourceSettings: %v", err)
	}
	body := rec.Body.String()
	// Fragment response shouldn't include the layout's <html> shell.
	if strings.Contains(body, "<html") {
		t.Error("HTMX response should not include full HTML layout")
	}
	if !strings.Contains(body, "Jan") {
		t.Errorf("expected fragment to contain card; got %d bytes", len(body))
	}
}
