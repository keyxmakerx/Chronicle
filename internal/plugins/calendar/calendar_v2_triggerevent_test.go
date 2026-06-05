// calendar_v2_triggerevent_test.go — C-CAL-WORLDSTATE-GM-LIVE-CONTROL-PANEL 4c.
// The trigger-world-event control persists a celestial event on the current
// day, its dm_only flag is capability-gated at the handler, the panel toggle +
// the V2 drawer's restricted-visibility editor are gated by CanAuthorDmOnly.
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

func TestSetWorldState_TriggerEventPersists(t *testing.T) {
	cal := gmTestCalendar() // current = 1492-6-15
	var got CelestialEvent
	repo := &mockCalendarRepo{
		getByIDFn:           func(_ context.Context, _ string) (*Calendar, error) { c := *cal; return &c, nil },
		addCelestialEventFn: func(_ context.Context, ce CelestialEvent) error { got = ce; return nil },
	}
	svc := NewCalendarService(repo)
	if err := svc.SetWorldState(context.Background(), "cal-1", WorldStateUpdateInput{
		TriggerEvent: &WorldStateTriggerEvent{Type: "eclipse-solar", Name: "Solar eclipse", StartHour: 22, DurationHours: 2, Visibility: "dm_only"},
	}); err != nil {
		t.Fatalf("SetWorldState trigger: %v", err)
	}
	if got.CalendarID != "cal-1" || got.Type != "eclipse-solar" || got.Visibility != "dm_only" ||
		got.Year != 1492 || got.Month != 6 || got.Day != 15 {
		t.Errorf("triggered event wrong: %+v", got)
	}

	// Unknown type is rejected before any write.
	called := false
	repo2 := &mockCalendarRepo{
		getByIDFn:           func(_ context.Context, _ string) (*Calendar, error) { c := *cal; return &c, nil },
		addCelestialEventFn: func(_ context.Context, _ CelestialEvent) error { called = true; return nil },
	}
	err := NewCalendarService(repo2).SetWorldState(context.Background(), "cal-1", WorldStateUpdateInput{
		TriggerEvent: &WorldStateTriggerEvent{Type: "definitely-not-real"},
	})
	if err == nil {
		t.Errorf("unknown celestial type should be rejected")
	}
	if called {
		t.Errorf("unknown type must not reach the repo")
	}
}

// TestPutWorldState_TriggerEventDmOnlyGate: the handler honors dm_only only for
// CanAuthorDmOnly holders; a non-author's dm_only request is downgraded.
func TestPutWorldState_TriggerEventDmOnlyGate(t *testing.T) {
	visFor := func(role campaigns.Role, dmGranted bool) string {
		cal := gmTestCalendar()
		var got string
		repo := &mockCalendarRepo{
			getByIDFn:           func(_ context.Context, _ string) (*Calendar, error) { c := *cal; return &c, nil },
			addCelestialEventFn: func(_ context.Context, ce CelestialEvent) error { got = ce.Visibility; return nil },
		}
		h := NewHandler(NewCalendarService(repo))
		e := echo.New()
		body := `{"triggerEvent":{"type":"blood-moon","name":"Blood moon","dm_only":true}}`
		req := httptest.NewRequest(http.MethodPut, "/x?calendarId=cal-1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("camp-1")
		c.Set("campaign_context", &campaigns.CampaignContext{
			Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role, IsDmGranted: dmGranted,
		})
		c.Set("auth_user_id", "user-1")
		if err := h.PutWorldState(c); err != nil {
			t.Fatalf("PutWorldState: %v", err)
		}
		return got
	}

	// Owner + co-DM → dm_only honored.
	if v := visFor(campaigns.RoleOwner, false); v != "dm_only" {
		t.Errorf("owner dm_only trigger should persist dm_only, got %q", v)
	}
	if v := visFor(campaigns.RoleScribe, true); v != "dm_only" {
		t.Errorf("co-DM dm_only trigger should persist dm_only, got %q", v)
	}
	// A non-author (would be route-blocked in prod; here we assert the
	// handler's own downgrade) → everyone.
	if v := visFor(campaigns.RoleScribe, false); v != "everyone" {
		t.Errorf("non-author dm_only trigger must downgrade to everyone, got %q", v)
	}
}

// TestGMPanel_TriggerEventDmOnlyGated: the panel's trigger control renders for
// holders; the dm_only toggle only when CanAuthorDmOnly.
func TestGMPanel_TriggerEventDmOnlyGated(t *testing.T) {
	render := func(canAuthor bool) string {
		data := CalendarV2ViewData{ActiveCalendar: gmTestCalendar(), CanControlWorldState: true, CanAuthorDmOnly: canAuthor}
		var sb strings.Builder
		if err := gmControlPanelV2(data).Render(context.Background(), &sb); err != nil {
			t.Fatalf("render panel: %v", err)
		}
		return sb.String()
	}
	withAuthor := render(true)
	if !strings.Contains(withAuthor, "data-gm-trigger-event") || !strings.Contains(withAuthor, "data-gm-event-dmonly") {
		t.Errorf("co-DM panel should have the trigger control + dm_only toggle")
	}
	noAuthor := render(false)
	if !strings.Contains(noAuthor, "data-gm-trigger-event") {
		t.Errorf("trigger control should still render (control = CanControlWorldState)")
	}
	if strings.Contains(noAuthor, "data-gm-event-dmonly") {
		t.Errorf("dm_only toggle must be hidden when !CanAuthorDmOnly")
	}
}

// TestEventDrawerV2_VisibilityGated: the V2 drawer's restricted-visibility
// editor renders only for CanAuthorDmOnly (the Phase-3 V2 follow-up); others
// get a locked "Visible to everyone".
func TestEventDrawerV2_VisibilityGated(t *testing.T) {
	render := func(canAuthor bool) string {
		data := CalendarV2ViewData{ActiveCalendar: &Calendar{ID: "cal-1", Name: "H"}, CanAuthorDmOnly: canAuthor}
		var sb strings.Builder
		if err := eventV2Drawer(data).Render(context.Background(), &sb); err != nil {
			t.Fatalf("render drawer: %v", err)
		}
		return sb.String()
	}
	if html := render(true); !strings.Contains(html, "data-visibility-editor") {
		t.Errorf("co-DM drawer should show the visibility editor")
	}
	if html := render(false); strings.Contains(html, "data-visibility-editor") || !strings.Contains(html, "data-event-visibility-locked") {
		t.Errorf("non-author drawer must lock visibility to everyone (no editor)")
	}
}
