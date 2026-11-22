package decks

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v5"
)

// recordingService answers every moment route as a service with no rules
// would, and counts the calls. A handler that reaches it for a player has
// dropped its own Director check; the same service answers a Director, so the
// control proves the route works.
type recordingService struct {
	Service
	calls atomic.Int64
}

func (s *recordingService) Moments(context.Context, string, Viewer) ([]MomentView, error) {
	s.calls.Add(1)
	return []MomentView{}, nil
}
func (s *recordingService) DealMoment(context.Context, string, string, Viewer) (*ActionResult, error) {
	s.calls.Add(1)
	return &ActionResult{}, nil
}
func (s *recordingService) AnotherMoment(context.Context, string, string, Viewer) (*MomentView, error) {
	s.calls.Add(1)
	return &MomentView{}, nil
}
func (s *recordingService) InviteMoment(context.Context, string, string, Viewer) (*MomentView, error) {
	s.calls.Add(1)
	return &MomentView{}, nil
}
func (s *recordingService) DismissMoment(context.Context, string, string, Viewer) (*MomentView, error) {
	s.calls.Add(1)
	return &MomentView{}, nil
}
func (s *recordingService) Say(context.Context, string, string, string, Viewer, bool) (DeckView, error) {
	s.calls.Add(1)
	return DirectorDeck{}, nil
}
func (s *recordingService) WhenOptions(context.Context, string, Viewer) (WhenOptionsView, error) {
	s.calls.Add(1)
	return WhenOptionsView{}, nil
}

type momentRoute struct{ method, path, body string }

var momentRoutes = []momentRoute{
	{http.MethodGet, "/campaigns/camp-1/decks/moments", ""},
	{http.MethodGet, "/campaigns/camp-1/decks/when-options", ""},
	{http.MethodGet, "/campaigns/camp-1/decks/moments/panel", ""},
	{http.MethodPost, "/campaigns/camp-1/decks/moments/m-1/deal", `{}`},
	{http.MethodPost, "/campaigns/camp-1/decks/moments/m-1/another", `{}`},
	{http.MethodPost, "/campaigns/camp-1/decks/moments/m-1/invite", `{}`},
	{http.MethodPost, "/campaigns/camp-1/decks/moments/m-1/dismiss", `{}`},
	{http.MethodPost, "/campaigns/camp-1/decks/d-1/cards/c-1/say", `{"said":true}`},
}

func newGateRig(t *testing.T, addonOn bool) (*echo.Echo, *recordingService) {
	t.Helper()
	fx := newFixture(t)
	rec := &recordingService{Service: fx.svc}
	e := echo.New()
	e.HTTPErrorHandler = newWorkshopRig(t, true).e.HTTPErrorHandler
	RegisterRoutes(e, NewHandler(rec), guardCampaigns{}, guardAuth{}, guardAddons{on: addonOn})
	return e, rec
}

func sendRoute(e *echo.Echo, r momentRoute, user string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(r.method, r.path, bytes.NewReader([]byte(r.body)))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	if user != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: user})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// Every new route turns a player and a scribe away with the same answer the
// workshop gives, before the service is asked for anything.
func TestEveryMomentRouteChecksForADirectorBeforeAskingTheService(t *testing.T) {
	e, svc := newGateRig(t, true)
	for _, r := range momentRoutes {
		svc.calls.Store(0)
		if rec := sendRoute(e, r, "u-dir"); rec.Code != http.StatusOK || svc.calls.Load() != 1 {
			t.Fatalf("the control: a Director at %s %s got %d after %d service calls", r.method, r.path, rec.Code, svc.calls.Load())
		}
		for _, user := range []string{"u-tam", "u-ilse", "u-scribe"} {
			svc.calls.Store(0)
			rec := sendRoute(e, r, user)
			if rec.Code != http.StatusNotFound || svc.calls.Load() != 0 {
				t.Errorf("%s at %s %s: %d after %d service calls, want 404 and none", user, r.method, r.path, rec.Code, svc.calls.Load())
			}
		}
	}
}

// A co-Director named by the campaign's grant is a Director for these routes.
func TestACoDirectorMayUseTheMomentRoutes(t *testing.T) {
	e, svc := newGateRig(t, true)
	for _, r := range momentRoutes {
		svc.calls.Store(0)
		if rec := sendRoute(e, r, "u-codm"); rec.Code != http.StatusOK || svc.calls.Load() != 1 {
			t.Errorf("a co-Director at %s %s: %d after %d calls", r.method, r.path, rec.Code, svc.calls.Load())
		}
	}
}

// Someone outside the campaign is told 403 by the campaign gate, a signed-out
// visitor is sent to sign in, and the service is never asked.
func TestMomentRoutesAnswerPeopleOutsideTheCampaign(t *testing.T) {
	e, svc := newGateRig(t, true)
	for _, r := range momentRoutes {
		if rec := sendRoute(e, r, "u-other"); rec.Code != http.StatusForbidden {
			t.Errorf("a non-member at %s %s: %d, want 403", r.method, r.path, rec.Code)
		}
		if rec := sendRoute(e, r, ""); rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
			t.Errorf("signed out at %s %s: %d to %q", r.method, r.path, rec.Code, rec.Header().Get("Location"))
		}
		if rec := sendRoute(e, r, "u-former"); rec.Code != http.StatusForbidden {
			t.Errorf("a former member at %s %s: %d, want 403", r.method, r.path, rec.Code)
		}
	}
	if svc.calls.Load() != 0 {
		t.Errorf("the service was asked %d times for people outside the campaign", svc.calls.Load())
	}
}

// With the Card decks add-on off every route is a 404 even for the Director.
func TestMomentRoutesAreOffWithTheAddonOff(t *testing.T) {
	e, svc := newGateRig(t, false)
	for _, r := range momentRoutes {
		for _, hdr := range [][]string{nil, {"Accept", "application/json"}, {"HX-Request", "true"}} {
			rec := sendRoute(e, r, "u-dir", hdr...)
			// A page navigation is sent back to the campaign; anything else is a 404.
			if rec.Code != http.StatusNotFound && !(len(hdr) == 0 && rec.Code == http.StatusSeeOther) {
				t.Errorf("%s %s with the add-on off %v: %d", r.method, r.path, hdr, rec.Code)
			}
		}
	}
	if svc.calls.Load() != 0 {
		t.Errorf("the service was asked %d times with the add-on off", svc.calls.Load())
	}
}

// The real service, not the recording one, refuses a player too, so a mis-wired
// route would still be stopped one layer down.
func TestTheServiceRefusesMomentCallsFromPlayersToo(t *testing.T) {
	fx := newFixture(t)
	for name, v := range map[string]Viewer{"player": fx.tam, "scribe": fx.scribe, "visitor": fx.visitor} {
		if _, err := fx.svc.Moments(bg, camp, v); code(err) != http.StatusNotFound {
			t.Errorf("%s Moments: %v", name, err)
		}
		if _, err := fx.svc.DealMoment(bg, camp, "m", v); code(err) != http.StatusNotFound {
			t.Errorf("%s DealMoment: %v", name, err)
		}
		if _, err := fx.svc.WhenOptions(bg, camp, v); code(err) == 0 {
			t.Errorf("%s WhenOptions was answered", name)
		}
	}
}
