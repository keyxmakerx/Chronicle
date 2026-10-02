// accent_hub_test.go pins the UpdateAccentColorAPI slot routing: no slot →
// chrome accent; slot 1/2 → the surface pair; bad slot → 400.
package campaigns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// accentRoutingService records which accent method the handler dispatched to.
// It embeds CampaignService so only the methods under test are implemented;
// the handler's accent path touches nothing else (logAudit is nil-safe).
type accentRoutingService struct {
	CampaignService
	colorCalled   bool
	colorValue    string
	surfaceCalled bool
	surfaceSlot   int
	surfaceValue  string
	actionCalled  bool
	actionValue   string
	appCalled     bool
	appValue      string
}

func (m *accentRoutingService) UpdateAccentColor(_ context.Context, _, color string) error {
	m.colorCalled = true
	m.colorValue = color
	return nil
}

func (m *accentRoutingService) UpdateAccentSurface(_ context.Context, _ string, slot int, color string) error {
	m.surfaceCalled = true
	m.surfaceSlot = slot
	m.surfaceValue = color
	return nil
}

func (m *accentRoutingService) UpdateAccentAction(_ context.Context, _, color string) error {
	m.actionCalled = true
	m.actionValue = color
	return nil
}

func (m *accentRoutingService) UpdateAccentApp(_ context.Context, _, color string) error {
	m.appCalled = true
	m.appValue = color
	return nil
}

// invokeAccentAPI runs UpdateAccentColorAPI with a form body against an Owner
// campaign context and returns the recording mock + any handler error.
func invokeAccentAPI(t *testing.T, form string) (*accentRoutingService, error) {
	t.Helper()
	svc := &accentRoutingService{}
	h := &Handler{service: svc}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/campaigns/camp-1/accent-color", strings.NewReader(form))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyCampaign, &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: RoleOwner})

	return svc, h.UpdateAccentColorAPI(c)
}

func TestUpdateAccentColorAPI_SlotRouting(t *testing.T) {
	// %23 = "#" url-encoded in the form body.
	t.Run("no slot routes to the chrome accent", func(t *testing.T) {
		svc, err := invokeAccentAPI(t, "accent_color=%23112233")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !svc.colorCalled || svc.surfaceCalled {
			t.Errorf("no slot must call UpdateAccentColor only (chrome=%v surface=%v)", svc.colorCalled, svc.surfaceCalled)
		}
		if svc.colorValue != "#112233" {
			t.Errorf("chrome color = %q, want #112233", svc.colorValue)
		}
	})

	t.Run("slot=1 routes to surface slot 1", func(t *testing.T) {
		svc, err := invokeAccentAPI(t, "accent_color=%23112233&slot=1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !svc.surfaceCalled || svc.colorCalled || svc.surfaceSlot != 1 {
			t.Errorf("slot=1 must call UpdateAccentSurface(slot=1) only (surface=%v slot=%d chrome=%v)",
				svc.surfaceCalled, svc.surfaceSlot, svc.colorCalled)
		}
	})

	t.Run("slot=2 routes to surface slot 2", func(t *testing.T) {
		svc, err := invokeAccentAPI(t, "accent_color=%23112233&slot=2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if svc.surfaceSlot != 2 {
			t.Errorf("slot=2 must map to surface slot 2, got %d", svc.surfaceSlot)
		}
	})

	t.Run("slot=action routes to the action-highlight accent", func(t *testing.T) {
		svc, err := invokeAccentAPI(t, "accent_color=%23112233&slot=action")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !svc.actionCalled || svc.colorCalled || svc.surfaceCalled || svc.appCalled {
			t.Errorf("slot=action must call UpdateAccentAction only (action=%v color=%v surface=%v app=%v)",
				svc.actionCalled, svc.colorCalled, svc.surfaceCalled, svc.appCalled)
		}
		if svc.actionValue != "#112233" {
			t.Errorf("action color = %q, want #112233", svc.actionValue)
		}
	})

	t.Run("slot=app routes to the app accent", func(t *testing.T) {
		svc, err := invokeAccentAPI(t, "accent_color=%23112233&slot=app")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !svc.appCalled || svc.colorCalled || svc.surfaceCalled || svc.actionCalled {
			t.Errorf("slot=app must call UpdateAccentApp only (app=%v color=%v surface=%v action=%v)",
				svc.appCalled, svc.colorCalled, svc.surfaceCalled, svc.actionCalled)
		}
		if svc.appValue != "#112233" {
			t.Errorf("app color = %q, want #112233", svc.appValue)
		}
	})

	t.Run("invalid slot is a 400 and dispatches nothing", func(t *testing.T) {
		svc, err := invokeAccentAPI(t, "accent_color=%23112233&slot=9")
		assertAppError(t, err, http.StatusBadRequest)
		if svc.colorCalled || svc.surfaceCalled || svc.actionCalled || svc.appCalled {
			t.Errorf("an invalid slot must not reach any service method")
		}
	})

	t.Run("invalid hex is a 400 and dispatches nothing", func(t *testing.T) {
		svc, err := invokeAccentAPI(t, "accent_color=red")
		assertAppError(t, err, http.StatusBadRequest)
		if svc.colorCalled || svc.surfaceCalled || svc.actionCalled || svc.appCalled {
			t.Errorf("an invalid hex color must be rejected before any service call")
		}
	})

	t.Run("invalid hex on slot=action is a 400 and dispatches nothing", func(t *testing.T) {
		svc, err := invokeAccentAPI(t, "accent_color=red&slot=action")
		assertAppError(t, err, http.StatusBadRequest)
		if svc.actionCalled {
			t.Errorf("an invalid hex color must be rejected before the action service call")
		}
	})

	t.Run("invalid hex on slot=app is a 400 and dispatches nothing", func(t *testing.T) {
		svc, err := invokeAccentAPI(t, "accent_color=red&slot=app")
		assertAppError(t, err, http.StatusBadRequest)
		if svc.appCalled {
			t.Errorf("an invalid hex color must be rejected before the app service call")
		}
	})
}
