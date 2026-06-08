// handler_test.go — C-WIDGET-BINDING-P4a. The binding HTTP surface:
// picker render (resolved source + instance cards + current highlight),
// bind / create+bind / unbind mutations (campaign from the ROUTE not the body,
// reload signalled), and the registry/host-type validation guard.
//
// Role gating (Scribe+) is middleware-enforced at the route layer
// (campaigns.RequireRole in routes.go) and exercised by the campaigns package;
// these tests cover the handler logic + the app-code namespace validation.
package widgetbindings

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- fakes ---

// fakeWT is a registry widget type with a real picker (ListInstances /
// CreateInstance) so the handler can exercise the P4a flows.
type fakeWT struct {
	slug      string
	instances []InstanceRef
	listErr   error
	created   []string // captured CreateInstance names
	newID     string   // id returned by CreateInstance
}

func (f *fakeWT) Slug() string { return f.slug }
func (f *fakeWT) InstanceExists(context.Context, string, string) (bool, error) {
	return true, nil
}
func (f *fakeWT) DefaultInstance(context.Context, HostRef) (string, bool, error) {
	return "", false, nil
}
func (f *fakeWT) ListInstances(_ context.Context, _ string, _ int) ([]InstanceRef, error) {
	return f.instances, f.listErr
}
func (f *fakeWT) CreateInstance(_ context.Context, _ string, input any) (string, error) {
	if ci, ok := input.(CreateInput); ok {
		f.created = append(f.created, ci.Name)
	}
	return f.newID, nil
}

// stubSvc records the binding mutations + serves a canned Resolution.
type stubSvc struct {
	resolution Resolution
	bound      []bindCall
	unbound    []HostRef
}

type bindCall struct {
	host       HostRef
	widgetType string
	instanceID string
}

func (s *stubSvc) Bind(_ context.Context, host HostRef, widgetType, instanceID string) error {
	s.bound = append(s.bound, bindCall{host, widgetType, instanceID})
	return nil
}
func (s *stubSvc) Unbind(_ context.Context, host HostRef, _ string) error {
	s.unbound = append(s.unbound, host)
	return nil
}
func (s *stubSvc) Resolve(context.Context, HostRef, string) (Resolution, error) {
	return s.resolution, nil
}
func (s *stubSvc) OnInstanceDeleted(context.Context, string, string, string) (int, error) {
	return 0, nil
}
func (s *stubSvc) Sweep(context.Context, string) (int, error) { return 0, nil }

// testHandler wires a handler over the stub service + a registry holding fakeWT
// under "calendar".
func testHandler(svc Service, wt *fakeWT) *Handler {
	reg := NewRegistry()
	if wt != nil {
		reg.Register(wt)
	}
	return NewHandler(svc, reg)
}

// hctx builds an Echo context with a Scribe campaign context (camp-1) + a CSRF
// token, for the given method/target/body.
func hctx(e *echo.Echo, method, target, body string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe,
	})
	c.Set("csrf_token", "tok-1")
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	return c, rec
}

// --- tests ---

// PickerAPI renders the generic picker with the resolved source, the instance
// cards, and the current instance highlighted.
func TestPickerAPI_RendersResolvedSourceAndInstances(t *testing.T) {
	svc := &stubSvc{resolution: Resolution{InstanceID: "cal-A", Source: SourceOwn, WidgetType: "calendar"}}
	wt := &fakeWT{slug: "calendar", instances: []InstanceRef{
		{ID: "cal-A", Name: "Harptos", Icon: "fa-calendar-days"},
		{ID: "cal-B", Name: "Earth", Icon: "fa-clock"},
	}}
	h := testHandler(svc, wt)
	e := echo.New()
	c, rec := hctx(e, http.MethodGet,
		"/campaigns/camp-1/bindings/picker?host_type=entity&host_id=ent-1&widget_type=calendar", "")

	if err := h.PickerAPI(c); err != nil {
		t.Fatalf("PickerAPI: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
	body := rec.Body.String()
	// Both instances render as cards.
	if !strings.Contains(body, "Harptos") || !strings.Contains(body, "Earth") {
		t.Errorf("picker missing instance cards: %s", body)
	}
	// The resolved source is surfaced (own → "Bound").
	if !strings.Contains(body, `data-binding-source="own"`) {
		t.Errorf("picker should expose resolved source own; body: %s", body)
	}
	// The current instance is highlighted (ring on cal-A) + "Current" label.
	if !strings.Contains(body, `data-binding-instance="cal-A"`) || !strings.Contains(body, "Current") {
		t.Errorf("current instance should be highlighted: %s", body)
	}
	// An explicit binding (own) shows the "use campaign default" revert control.
	if !strings.Contains(body, "Use campaign default") {
		t.Errorf("own binding should offer revert-to-default; body: %s", body)
	}
}

// When the host is on the default (no explicit binding), the picker shows the
// default source and omits the revert control.
func TestPickerAPI_DefaultSourceHidesRevert(t *testing.T) {
	svc := &stubSvc{resolution: Resolution{Source: SourceDefault, WidgetType: "calendar"}}
	h := testHandler(svc, &fakeWT{slug: "calendar"})
	e := echo.New()
	c, rec := hctx(e, http.MethodGet,
		"/campaigns/camp-1/bindings/picker?host_type=entity&host_id=ent-1&widget_type=calendar", "")

	if err := h.PickerAPI(c); err != nil {
		t.Fatalf("PickerAPI: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-binding-source="default"`) {
		t.Errorf("expected default source; body: %s", body)
	}
	if strings.Contains(body, "Use campaign default") {
		t.Errorf("default source must NOT offer revert; body: %s", body)
	}
}

// An unknown host_type / widget_type is a 400 (registry-validated, no DB enum).
func TestPickerAPI_RejectsUnknownTypes(t *testing.T) {
	h := testHandler(&stubSvc{}, &fakeWT{slug: "calendar"})
	e := echo.New()
	cases := []string{
		"/campaigns/camp-1/bindings/picker?host_type=entity&host_id=ent-1&widget_type=nope",    // unknown widget
		"/campaigns/camp-1/bindings/picker?host_type=bogus&host_id=ent-1&widget_type=calendar", // bad host type
		"/campaigns/camp-1/bindings/picker?host_type=entity&host_id=&widget_type=calendar",     // missing host id
	}
	for _, target := range cases {
		c, _ := hctx(e, http.MethodGet, target, "")
		err := h.PickerAPI(c)
		if !isBadRequest(err) {
			t.Errorf("%s: want 400 bad request; got %v", target, err)
		}
	}
}

// BindAPI binds the host to an instance using the ROUTE campaign (never the
// body) and signals a reload.
func TestBindAPI_UsesRouteCampaignAndReloads(t *testing.T) {
	svc := &stubSvc{}
	h := testHandler(svc, &fakeWT{slug: "calendar"})
	e := echo.New()
	form := url.Values{
		"host_type":      {"entity"},
		"host_id":        {"ent-1"},
		"entity_type_id": {"42"},
		"widget_type":    {"calendar"},
		"instance_id":    {"cal-B"},
		// A spoofed campaign in the body must be IGNORED.
		"campaign_id": {"camp-EVIL"},
	}
	c, rec := hctx(e, http.MethodPost, "/campaigns/camp-1/bindings", form.Encode())

	if err := h.BindAPI(c); err != nil {
		t.Fatalf("BindAPI: %v", err)
	}
	if len(svc.bound) != 1 {
		t.Fatalf("want 1 bind; got %d", len(svc.bound))
	}
	got := svc.bound[0]
	if got.host.CampaignID != "camp-1" {
		t.Errorf("campaign must come from the route, got %q", got.host.CampaignID)
	}
	if got.host.Type != "entity" || got.host.ID != "ent-1" || got.host.EntityTypeID != "42" {
		t.Errorf("host mis-bound: %+v", got.host)
	}
	if got.widgetType != "calendar" || got.instanceID != "cal-B" {
		t.Errorf("bind args = %+v", got)
	}
	if rec.Header().Get("HX-Refresh") != "true" {
		t.Errorf("bind should signal HX-Refresh; headers: %v", rec.Header())
	}
}

// CreateBindAPI creates a new instance (name from the form) then binds it.
func TestCreateBindAPI_CreatesThenBinds(t *testing.T) {
	svc := &stubSvc{}
	wt := &fakeWT{slug: "calendar", newID: "cal-new"}
	h := testHandler(svc, wt)
	e := echo.New()
	form := url.Values{
		"host_type":   {"entity"},
		"host_id":     {"ent-1"},
		"widget_type": {"calendar"},
		"name":        {"Harptos"},
	}
	c, rec := hctx(e, http.MethodPost, "/campaigns/camp-1/bindings/create", form.Encode())

	if err := h.CreateBindAPI(c); err != nil {
		t.Fatalf("CreateBindAPI: %v", err)
	}
	if len(wt.created) != 1 || wt.created[0] != "Harptos" {
		t.Errorf("create should pass the form name; got %v", wt.created)
	}
	if len(svc.bound) != 1 || svc.bound[0].instanceID != "cal-new" {
		t.Errorf("should bind the freshly-created instance; got %+v", svc.bound)
	}
	if svc.bound[0].host.CampaignID != "camp-1" {
		t.Errorf("create+bind must use the route campaign; got %q", svc.bound[0].host.CampaignID)
	}
	if rec.Header().Get("HX-Refresh") != "true" {
		t.Errorf("create+bind should signal HX-Refresh")
	}
}

// UnbindAPI removes the host's binding (reverting to default) and reloads.
func TestUnbindAPI_RemovesBindingAndReloads(t *testing.T) {
	svc := &stubSvc{}
	h := testHandler(svc, &fakeWT{slug: "calendar"})
	e := echo.New()
	c, rec := hctx(e, http.MethodDelete,
		"/campaigns/camp-1/bindings?host_type=entity&host_id=ent-1&widget_type=calendar", "")

	if err := h.UnbindAPI(c); err != nil {
		t.Fatalf("UnbindAPI: %v", err)
	}
	if len(svc.unbound) != 1 || svc.unbound[0].CampaignID != "camp-1" || svc.unbound[0].ID != "ent-1" {
		t.Errorf("unbind host = %+v", svc.unbound)
	}
	if rec.Header().Get("HX-Refresh") != "true" {
		t.Errorf("unbind should signal HX-Refresh")
	}
}

// Mutations also reject unknown types before touching the service.
func TestMutations_RejectUnknownTypes(t *testing.T) {
	svc := &stubSvc{}
	h := testHandler(svc, &fakeWT{slug: "calendar"})
	e := echo.New()

	bindForm := url.Values{"host_type": {"entity"}, "host_id": {"ent-1"}, "widget_type": {"nope"}, "instance_id": {"x"}}.Encode()
	cBind, _ := hctx(e, http.MethodPost, "/campaigns/camp-1/bindings", bindForm)
	if err := h.BindAPI(cBind); !isBadRequest(err) {
		t.Errorf("bind unknown widget → 400; got %v", err)
	}

	cUnbind, _ := hctx(e, http.MethodDelete,
		"/campaigns/camp-1/bindings?host_type=bogus&host_id=ent-1&widget_type=calendar", "")
	if err := h.UnbindAPI(cUnbind); !isBadRequest(err) {
		t.Errorf("unbind bad host type → 400; got %v", err)
	}

	if len(svc.bound) != 0 || len(svc.unbound) != 0 {
		t.Errorf("rejected mutations must not reach the service; bound=%v unbound=%v", svc.bound, svc.unbound)
	}
}

// isBadRequest reports whether err is a 400 AppError.
func isBadRequest(err error) bool {
	if err == nil {
		return false
	}
	if ae, ok := err.(*apperror.AppError); ok {
		return ae.Code == http.StatusBadRequest
	}
	return false
}
