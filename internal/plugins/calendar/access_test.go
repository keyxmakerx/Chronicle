// access_test.go: full-stack route tests through RegisterRoutes (real
// e.ServeHTTP, the actual middleware chain) — the harness mirrors
// internal/plugins/maps/anonymous_access_test.go (guard*Svc fakes embedding
// the real service interfaces) extended with a minimal fake session/
// membership flow so authenticated per-role requests can be exercised too,
// not just anonymous ones.
//
// Covers the route-level security rules: a Player can't reach Owner-only or
// Scribe-only routes; a non-member of a private campaign gets nothing; a
// public campaign's anonymous visitor can read only what RequireViewAccess
// allows and never dm_only content; a disabled calendar addon answers the
// way maps answers with its addon off.
package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- Fakes for the middleware chain (auth, campaigns, addons) ---

// sessionCookieName mirrors auth's bare (non-__Host-) cookie name; these
// tests run over plain HTTP (httptest, no TLS), which is the branch
// auth.getSessionToken reads.
const sessionCookieName = "chronicle_session"

// guardAuthSvc's ValidateSession treats the token AS the user id — good
// enough for a test harness that only needs a stable identity, never real
// credential verification.
type guardAuthSvc struct{ auth.AuthService }

func (guardAuthSvc) ValidateSession(_ context.Context, token string) (*auth.Session, error) {
	if token == "" {
		return nil, apperror.NewUnauthorized("no token")
	}
	return &auth.Session{UserID: token}, nil
}

type guardAddonSvc struct {
	addons.AddonService
	enabled bool
}

func (g guardAddonSvc) IsEnabledForCampaign(_ context.Context, _, _ string) (bool, error) {
	return g.enabled, nil
}

// guardCampaignSvc resolves membership from a fixed userID->Role map, so a
// test declares "u-player is a Player" etc. up front. An id absent from
// roles is treated as a genuine non-member (GetMember errors), matching
// resolveCampaignContext's real "not a member" branch.
type guardCampaignSvc struct {
	campaigns.CampaignService
	public bool
	roles  map[string]campaigns.Role
	// settings is the campaign's raw Settings JSON, e.g. `{"dm_grant_ids":["u-codm"]}`
	// — a co-DM test wires this so CampaignContext.IsDmGranted (and so
	// CanAuthorDmOnly/VisibilityRole) resolves the same way it does in
	// production, off the real hasDmGrant/ParseSettings path.
	settings string
}

func (g guardCampaignSvc) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	return &campaigns.Campaign{ID: id, IsPublic: g.public, Settings: g.settings}, nil
}

func (g guardCampaignSvc) GetMember(_ context.Context, _, userID string) (*campaigns.CampaignMember, error) {
	role, ok := g.roles[userID]
	if !ok {
		return nil, apperror.NewNotFound("not a member")
	}
	return &campaigns.CampaignMember{UserID: userID, Role: role}, nil
}

// --- Fake CalendarService ---

// fakeCalendarSvc implements CalendarService with just enough behavior to
// prove routing/gating and the viewer built at the HTTP boundary, without
// re-testing the business logic service_test.go already covers against real
// fakes-of-repositories. lastViewer captures the most recent viewer a read
// method received, so a test can assert the handler built it correctly
// (role, user id, anonymity) straight from the HTTP request.
type fakeCalendarSvc struct {
	lastViewer permissions.Viewer
	// weather is what GetWeatherSettings reports (defaults when nil);
	// savedWeather records the last SetWeatherSettings.
	weather      *WeatherSettings
	savedWeather *WeatherSettings
}

const secretCalendarID = "cal-secret"

func (f *fakeCalendarSvc) CreateCalendar(_ context.Context, campaignID string, input CreateCalendarInput) (*Calendar, error) {
	return &Calendar{ID: "cal-new", CampaignID: campaignID, Name: input.Name}, nil
}
func (f *fakeCalendarSvc) GetCalendarForViewer(_ context.Context, calendarID, campaignID string, v permissions.Viewer) (*Calendar, error) {
	f.lastViewer = v
	if calendarID == secretCalendarID && !v.SkipsPerUserRules() {
		return nil, apperror.NewNotFound("calendar not found")
	}
	return &Calendar{ID: calendarID, CampaignID: campaignID, Name: "The Secret Calendar"}, nil
}
func (f *fakeCalendarSvc) GetDefaultCalendarForViewer(_ context.Context, campaignID string, v permissions.Viewer) (*Calendar, error) {
	f.lastViewer = v
	return &Calendar{ID: "cal-default", CampaignID: campaignID, Name: "The Default Calendar", IsDefault: true}, nil
}
func (f *fakeCalendarSvc) ListCalendars(_ context.Context, _ string, v permissions.Viewer) ([]Calendar, error) {
	f.lastViewer = v
	return []Calendar{{ID: "cal-1"}}, nil
}
func (f *fakeCalendarSvc) SetCurrentDate(context.Context, string, string, int, int, int, int, int) error {
	return nil
}
func (f *fakeCalendarSvc) UpdateCalendar(context.Context, string, string, UpdateCalendarInput) error {
	return nil
}
func (f *fakeCalendarSvc) DeleteCalendar(context.Context, string, string) error     { return nil }
func (f *fakeCalendarSvc) SetDefaultCalendar(context.Context, string, string) error { return nil }

func (f *fakeCalendarSvc) CreateEvent(_ context.Context, calendarID, campaignID string, input CreateEventInput) (*Event, error) {
	return &Event{ID: "evt-new", CalendarID: calendarID, Name: input.Name}, nil
}
func (f *fakeCalendarSvc) GetEventForViewer(_ context.Context, eventID, calendarID, _ string, v permissions.Viewer) (*Event, error) {
	f.lastViewer = v
	if eventID == secretCalendarID && !v.SkipsPerUserRules() {
		return nil, apperror.NewNotFound("event not found")
	}
	return &Event{ID: eventID, CalendarID: calendarID, Name: "The Secret Event"}, nil
}
func (f *fakeCalendarSvc) ListEventsByIDsForViewer(context.Context, string, string, []string, permissions.Viewer) ([]Event, error) {
	return nil, nil
}
func (f *fakeCalendarSvc) GetCalendarNameForViewer(context.Context, string, string, permissions.Viewer) (string, error) {
	return "", nil
}
func (f *fakeCalendarSvc) ListEventsForMonth(_ context.Context, calendarID, _ string, _, _ int, v permissions.Viewer) ([]Event, error) {
	f.lastViewer = v
	return []Event{{ID: "evt-1", CalendarID: calendarID}}, nil
}
func (f *fakeCalendarSvc) ListUpcomingEvents(_ context.Context, calendarID, _ string, _ int, v permissions.Viewer) ([]Event, error) {
	f.lastViewer = v
	return []Event{{ID: "evt-1", CalendarID: calendarID}}, nil
}
func (f *fakeCalendarSvc) UpdateEvent(_ context.Context, _, _, _ string, _ UpdateEventInput, v permissions.Viewer) error {
	f.lastViewer = v
	return nil
}
func (f *fakeCalendarSvc) DeleteEvent(_ context.Context, _, _, _ string, v permissions.Viewer) error {
	f.lastViewer = v
	return nil
}
func (f *fakeCalendarSvc) SetEventVisibility(_ context.Context, _, _, _ string, _ UpdateEventVisibilityInput, v permissions.Viewer) error {
	f.lastViewer = v
	return nil
}

func (f *fakeCalendarSvc) ListEventKinds(context.Context, string) ([]EventKind, error) {
	return nil, nil
}
func (f *fakeCalendarSvc) CreateEventKind(_ context.Context, campaignID string, input EventKindInput) (*EventKind, error) {
	return &EventKind{ID: 1, CampaignID: campaignID, Slug: input.Slug, Name: input.Name}, nil
}
func (f *fakeCalendarSvc) UpdateEventKind(context.Context, int, string, UpdateEventKindInput) error {
	return nil
}
func (f *fakeCalendarSvc) DeleteEventKind(context.Context, int, string) error { return nil }

func (f *fakeCalendarSvc) CreateEra(_ context.Context, calendarID, _ string, input EraInput) (*Era, error) {
	return &Era{ID: 1, CalendarID: calendarID, Name: input.Name}, nil
}
func (f *fakeCalendarSvc) UpdateEra(context.Context, int, string, string, UpdateEraInput) error {
	return nil
}
func (f *fakeCalendarSvc) DeleteEra(context.Context, int, string, string) error { return nil }
func (f *fakeCalendarSvc) SaveEraLook(context.Context, string, string, EraLook, []EraLookEra) error {
	return nil
}

func (f *fakeCalendarSvc) SetMoonHidden(context.Context, int, string, string, bool) error { return nil }

func (f *fakeCalendarSvc) SetMonths(context.Context, string, string, []MonthInput) error { return nil }
func (f *fakeCalendarSvc) SetWeekdays(context.Context, string, string, []WeekdayInput) error {
	return nil
}
func (f *fakeCalendarSvc) SetMoons(context.Context, string, string, []MoonInput) error { return nil }
func (f *fakeCalendarSvc) SetSeasons(context.Context, string, string, []Season) error  { return nil }
func (f *fakeCalendarSvc) SetCycles(context.Context, string, string, []CycleInput) error {
	return nil
}
func (f *fakeCalendarSvc) SetFestivals(context.Context, string, string, []FestivalInput) error {
	return nil
}
func (f *fakeCalendarSvc) SetWeather(context.Context, string, string, WeatherInput) error {
	return nil
}
func (f *fakeCalendarSvc) ListDayWeather(context.Context, string, string, int, int, permissions.Viewer) ([]DayWeather, error) {
	return nil, nil
}
func (f *fakeCalendarSvc) SetDayWeather(context.Context, string, string, []DayWeatherInput) error {
	return nil
}
func (f *fakeCalendarSvc) GetWeatherSettings(context.Context, string, string, permissions.Viewer) (*WeatherSettings, error) {
	if f.weather != nil {
		w := *f.weather
		return &w, nil
	}
	return &WeatherSettings{Climate: DefaultWeatherClimate, Continuity: DefaultWeatherContinuity}, nil
}
func (f *fakeCalendarSvc) SetWeatherSettings(_ context.Context, _, _ string, s WeatherSettings) error {
	f.savedWeather = &s
	return nil
}
func (f *fakeCalendarSvc) ClearDayWeather(context.Context, string, string, []DayDate) error {
	return nil
}

func (f *fakeCalendarSvc) ListAllEventsForCalendar(context.Context, string, string, permissions.Viewer) ([]Event, error) {
	return nil, nil
}

func (f *fakeCalendarSvc) SearchCalendarEvents(context.Context, string, string, int) ([]map[string]string, error) {
	return nil, nil
}

func (f *fakeCalendarSvc) ListEventsForCalendar(context.Context, string, string, int) ([]Event, error) {
	return nil, nil
}

func (f *fakeCalendarSvc) ListErasForCalendar(context.Context, string, string, int) ([]Era, error) {
	return nil, nil
}

func (f *fakeCalendarSvc) SetOccurrenceOverride(_ context.Context, _, _, _ string, _ DayDate, _ OccurrenceOverrideInput, v permissions.Viewer) (*OccurrenceOverride, error) {
	f.lastViewer = v
	return &OccurrenceOverride{}, nil
}

func (f *fakeCalendarSvc) DeleteOccurrenceOverride(_ context.Context, _, _, _ string, _ DayDate, v permissions.Viewer) error {
	f.lastViewer = v
	return nil
}

func (f *fakeCalendarSvc) PreviewRecurrence(_ context.Context, _, _ string, _ RecurrencePreviewInput, v permissions.Viewer) (*RecurrencePreview, error) {
	f.lastViewer = v
	return &RecurrencePreview{Dates: []DayDate{}}, nil
}

func (f *fakeCalendarSvc) ListOccurrenceOverrides(context.Context, string, string, permissions.Viewer) (map[string][]OccurrenceOverride, error) {
	return nil, nil
}

func (f *fakeCalendarSvc) PreviewImport(context.Context, []byte) (*ImportResult, error) {
	return &ImportResult{Format: FormatChronicle, CalendarName: "Previewed Calendar"}, nil
}
func (f *fakeCalendarSvc) PreviewPreset(_ context.Context, name string) (*ImportResult, error) {
	return &ImportResult{Format: FormatChronicle, CalendarName: "Preset " + name}, nil
}
func (f *fakeCalendarSvc) PreviewRealWorld(context.Context) (*ImportResult, error) {
	return GregorianImportResult()
}
func (f *fakeCalendarSvc) TodayInZone(string) (int, int, int, error) { return 2026, 1, 1, nil }
func (f *fakeCalendarSvc) CreateCalendarFromImport(_ context.Context, campaignID string, ir *ImportResult, _ CreateCalendarFromImportOptions) (*Calendar, error) {
	return &Calendar{ID: "cal-imported", CampaignID: campaignID, Name: ir.CalendarName}, nil
}
func (f *fakeCalendarSvc) PreviewStructureEdit(context.Context, string, string, StructureEdit) (*StructurePreview, error) {
	return &StructurePreview{CalendarName: "The Secret Calendar", Fingerprint: "fp"}, nil
}
func (f *fakeCalendarSvc) ApplyStructureEdit(context.Context, string, string, string, StructureEdit) (*StructurePreview, error) {
	return &StructurePreview{CalendarName: "The Secret Calendar", Fingerprint: "fp"}, nil
}
func (f *fakeCalendarSvc) PreviewAnchorMove(context.Context, string, string, int, int, int, time.Time) (*AnchorMovePreview, error) {
	return &AnchorMovePreview{}, nil
}

// --- Harness ---

func newAccessTestRouter(public, addonEnabled bool, roles map[string]campaigns.Role) (*echo.Echo, *fakeCalendarSvc) {
	return newAccessTestRouterWithSettings(public, addonEnabled, roles, "")
}

// newAccessTestRouterWithSettings is newAccessTestRouter plus the campaign's
// raw Settings JSON, for a test that needs a co-DM grant (guardCampaignSvc's
// doc comment).
func newAccessTestRouterWithSettings(public, addonEnabled bool, roles map[string]campaigns.Role, settings string) (*echo.Echo, *fakeCalendarSvc) {
	e := echo.New()
	e.Use(emw.Recover())
	// The real app's HTTPErrorHandler lives in internal/app (unexported,
	// and importing it here would cycle back through this package). This
	// minimal stand-in translates *apperror.AppError to its own status code
	// exactly like it does — the same shape internal/plugins/npcs/
	// public_route_test.go uses for the identical reason.
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.JSON(ae.Code, map[string]string{"error": ae.Type, "message": ae.Message})
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	svc := &fakeCalendarSvc{}
	h := NewHandler(svc)
	campaignSvc := guardCampaignSvc{public: public, roles: roles, settings: settings}
	RegisterRoutes(e, h, campaignSvc, guardAuthSvc{}, guardAddonSvc{enabled: addonEnabled})
	return e, svc
}

func doRequest(e *echo.Echo, method, path, userID string) *httptest.ResponseRecorder {
	return doRequestWithBody(e, method, path, userID, "")
}

// doRequestWithBody is doRequest plus a JSON body, for the write-path tests
// that must distinguish an absent key from a present one (patch.Field).
func doRequestWithBody(e *echo.Echo, method, path, userID, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if userID != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userID})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// isLoginRedirect mirrors maps/anonymous_access_test.go's helper: reports
// whether the response bounced the caller to the login page.
func isLoginRedirect(rec *httptest.ResponseRecorder) bool {
	switch rec.Code {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		loc := rec.Header().Get("Location")
		return loc == "/login" || strings.HasPrefix(loc, "/login?")
	}
	return false
}

// --- Role gates on the authenticated (private-campaign) group ---

func TestRouteGates_PlayerBlockedFromOwnerAndScribeRoutes(t *testing.T) {
	roles := map[string]campaigns.Role{"u-player": campaigns.RolePlayer}
	e, _ := newAccessTestRouter(false, true, roles)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"delete calendar (Owner only)", http.MethodDelete, "/campaigns/camp-1/calendars/cal-1"},
		{"set default calendar (Owner only)", http.MethodPut, "/campaigns/camp-1/calendars/cal-1/default"},
		{"create event (Scribe+)", http.MethodPost, "/campaigns/camp-1/calendars/cal-1/events"},
		{"update event (Scribe+)", http.MethodPut, "/campaigns/camp-1/calendars/cal-1/events/evt-1"},
		{"delete event (Owner only)", http.MethodDelete, "/campaigns/camp-1/calendars/cal-1/events/evt-1"},
		{"set event visibility (the dm_only toggle, gated on CanAuthorDmOnly)", http.MethodPut, "/campaigns/camp-1/calendars/cal-1/events/evt-1/visibility"},
		{"list event kinds (calendar structure, Owner only)", http.MethodGet, "/campaigns/camp-1/calendars/event-kinds"},
		{"create event kind (CanAuthorDmOnly)", http.MethodPost, "/campaigns/camp-1/calendars/event-kinds"},
		{"create era (CanAuthorDmOnly)", http.MethodPost, "/campaigns/camp-1/calendars/cal-1/eras"},
		{"set moon hidden (CanAuthorDmOnly)", http.MethodPut, "/campaigns/camp-1/calendars/cal-1/moons/1/hidden"},
		{"list calendar presets (Owner only)", http.MethodGet, "/campaigns/camp-1/calendars/presets"},
		{"preview calendar preset (Owner only)", http.MethodGet, "/campaigns/camp-1/calendars/presets/blank"},
		{"create calendar from preset (Owner only)", http.MethodPost, "/campaigns/camp-1/calendars/presets/blank"},
		{"preview calendar import (Owner only)", http.MethodPost, "/campaigns/camp-1/calendars/import/preview"},
		{"create calendar from import (Owner only)", http.MethodPost, "/campaigns/camp-1/calendars/import"},
		{"wizard start (Owner only)", http.MethodGet, "/campaigns/camp-1/calendars/wizard"},
		{"wizard preset picker (Owner only)", http.MethodGet, "/campaigns/camp-1/calendars/wizard/presets"},
		{"wizard preset review (Owner only)", http.MethodGet, "/campaigns/camp-1/calendars/wizard/presets/blank"},
		{"wizard import step (Owner only)", http.MethodGet, "/campaigns/camp-1/calendars/wizard/import"},
		{"wizard import preview (Owner only)", http.MethodPost, "/campaigns/camp-1/calendars/wizard/import/preview"},
		{"wizard create (Owner only)", http.MethodPost, "/campaigns/camp-1/calendars/wizard/create"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(e, tt.method, tt.path, "u-player")
			if rec.Code != http.StatusForbidden {
				t.Errorf("Player must be forbidden, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRouteGates_ScribeCanCreateAndEditEventsButNotDeleteOrToggleVisibility(t *testing.T) {
	roles := map[string]campaigns.Role{"u-scribe": campaigns.RoleScribe}
	e, _ := newAccessTestRouter(false, true, roles)

	rec := doRequest(e, http.MethodPost, "/campaigns/camp-1/calendars/cal-1/events", "u-scribe")
	if rec.Code != http.StatusCreated {
		t.Errorf("Scribe must be able to create an event, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPut, "/campaigns/camp-1/calendars/cal-1/events/evt-1", "u-scribe")
	if rec.Code != http.StatusOK {
		t.Errorf("Scribe must be able to update an event, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodDelete, "/campaigns/camp-1/calendars/cal-1/events/evt-1", "u-scribe")
	if rec.Code != http.StatusForbidden {
		t.Errorf("Scribe must NOT be able to delete an event, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPut, "/campaigns/camp-1/calendars/cal-1/events/evt-1/visibility", "u-scribe")
	if rec.Code != http.StatusForbidden {
		t.Errorf("Scribe must NOT be able to toggle the dm_only visibility, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRouteGates_OwnerReachesEveryRoute(t *testing.T) {
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner}
	e, _ := newAccessTestRouter(false, true, roles)

	tests := []struct {
		method, path string
		want         int
	}{
		{http.MethodDelete, "/campaigns/camp-1/calendars/cal-1", http.StatusOK},
		{http.MethodPut, "/campaigns/camp-1/calendars/cal-1/default", http.StatusOK},
		{http.MethodPost, "/campaigns/camp-1/calendars/event-kinds", http.StatusCreated},
		{http.MethodPost, "/campaigns/camp-1/calendars/cal-1/eras", http.StatusCreated},
		{http.MethodPut, "/campaigns/camp-1/calendars/cal-1/moons/1/hidden", http.StatusOK},
		{http.MethodPut, "/campaigns/camp-1/calendars/cal-1/events/evt-1/visibility", http.StatusOK},
		{http.MethodGet, "/campaigns/camp-1/calendars/presets", http.StatusOK},
		{http.MethodGet, "/campaigns/camp-1/calendars/presets/blank", http.StatusOK},
		{http.MethodPost, "/campaigns/camp-1/calendars/presets/blank", http.StatusCreated},
	}
	for _, tt := range tests {
		rec := doRequest(e, tt.method, tt.path, "u-owner")
		if rec.Code != tt.want {
			t.Errorf("%s %s: owner expected %d, got %d: %s", tt.method, tt.path, tt.want, rec.Code, rec.Body.String())
		}
	}
}

// TestRouteGates_CoDMCanToggleEventVisibility proves the dm_only toggle's
// route gate is CanAuthorDmOnly, not a bare role minimum: a co-DM (Scribe
// role, plus the operator's dm_only grant) reaches it exactly like an Owner,
// where a plain Scribe (see TestRouteGates_ScribeCanCreateAndEditEventsButNotDeleteOrToggleVisibility)
// is forbidden.
func TestRouteGates_CoDMCanToggleEventVisibility(t *testing.T) {
	roles := map[string]campaigns.Role{"u-codm": campaigns.RoleScribe}
	e, _ := newAccessTestRouterWithSettings(false, true, roles, `{"dm_grant_ids":["u-codm"]}`)

	rec := doRequest(e, http.MethodPut, "/campaigns/camp-1/calendars/cal-1/events/evt-1/visibility", "u-codm")
	if rec.Code != http.StatusOK {
		t.Errorf("a granted co-DM must be able to toggle event visibility, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestRouteGates_StructureWritesCanAuthorDmOnly is the required table for
// the era/event-kind/moon-hidden writes widened from RequireRole(RoleOwner)
// to CanAuthorDmOnly: for every one of the seven changed routes, only the
// Owner and a granted co-Director (Scribe role + the dm_grant_ids grant) may
// reach the fake service; a plain Scribe, a Player and a non-member all get
// 403, exactly as they did before the widening — only the co-Director's
// outcome flips from 403 to success.
func TestRouteGates_StructureWritesCanAuthorDmOnly(t *testing.T) {
	type caller struct {
		name    string
		role    campaigns.Role
		member  bool // false = not in the roles map at all ("no access")
		granted bool
	}
	callers := []caller{
		{"Owner", campaigns.RoleOwner, true, false},
		{"co-Director", campaigns.RoleScribe, true, true},
		{"Scribe", campaigns.RoleScribe, true, false},
		{"Player", campaigns.RolePlayer, true, false},
		{"no access", campaigns.RoleNone, false, false},
	}
	routes := []struct {
		name   string
		method string
		path   string
		wantOK int // response code for a caller who passes CanAuthorDmOnly
	}{
		{"create event kind", http.MethodPost, "/campaigns/camp-1/calendars/event-kinds", http.StatusCreated},
		{"update event kind", http.MethodPut, "/campaigns/camp-1/calendars/event-kinds/1", http.StatusOK},
		{"delete event kind", http.MethodDelete, "/campaigns/camp-1/calendars/event-kinds/1", http.StatusOK},
		{"create era", http.MethodPost, "/campaigns/camp-1/calendars/cal-1/eras", http.StatusCreated},
		{"update era", http.MethodPut, "/campaigns/camp-1/calendars/cal-1/eras/1", http.StatusOK},
		{"delete era", http.MethodDelete, "/campaigns/camp-1/calendars/cal-1/eras/1", http.StatusOK},
		{"set moon hidden", http.MethodPut, "/campaigns/camp-1/calendars/cal-1/moons/1/hidden", http.StatusOK},
		{"read weather settings", http.MethodGet, "/campaigns/camp-1/calendars/cal-1/weather/settings", http.StatusOK},
	}

	const userID = "u-caller"
	for _, rt := range routes {
		rt := rt
		t.Run(rt.name, func(t *testing.T) {
			for _, c := range callers {
				c := c
				t.Run(c.name, func(t *testing.T) {
					roles := map[string]campaigns.Role{}
					if c.member {
						roles[userID] = c.role
					}
					settings := ""
					if c.granted {
						settings = `{"dm_grant_ids":["` + userID + `"]}`
					}
					e, _ := newAccessTestRouterWithSettings(false, true, roles, settings)
					rec := doRequest(e, rt.method, rt.path, userID)

					canAuthor := c.role >= campaigns.RoleOwner || c.granted
					want := http.StatusForbidden
					if canAuthor {
						want = rt.wantOK
					}
					if rec.Code != want {
						t.Errorf("%s as %s: got %d, want %d: %s", rt.name, c.name, rec.Code, want, rec.Body.String())
					}
				})
			}
		})
	}
}

// --- Non-member of a private campaign ---

func TestRouteGates_NonMemberOfPrivateCampaignGetsNothing(t *testing.T) {
	e, _ := newAccessTestRouter(false, true, map[string]campaigns.Role{}) // "u-stranger" is in no role map

	rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/cal-1", "u-stranger")
	if rec.Code != http.StatusForbidden {
		t.Errorf("a non-member of a private campaign must be forbidden, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRouteGates_AnonymousOnPrivateCampaignBouncesToLogin(t *testing.T) {
	e, _ := newAccessTestRouter(false, true, nil)

	rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/cal-1", "")
	if !isLoginRedirect(rec) {
		t.Errorf("anonymous visitor to a private campaign must bounce to /login, got %d loc=%q body=%s",
			rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
}

// --- Public campaign: anonymous visitor ---

func TestRouteGates_PublicCampaign_AnonymousCanReadButNeverDMOnly(t *testing.T) {
	e, svc := newAccessTestRouter(true, true, nil)

	// Ordinary read: allowed, and the viewer built for it must be a genuine
	// anonymous viewer (empty user id, RoleNone, never SkipsPerUserRules).
	rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/cal-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("anonymous visitor on a public campaign must be able to read a calendar, got %d: %s", rec.Code, rec.Body.String())
	}
	if !svc.lastViewer.IsAnonymous() || svc.lastViewer.SkipsPerUserRules() {
		t.Errorf("handler must build a genuine anonymous viewer, got role=%d userID=%q isSystem=%v",
			svc.lastViewer.Role(), svc.lastViewer.UserID(), svc.lastViewer.IsSystem())
	}

	// The dm_only fixture: never leaks to anonymous, even on a public campaign.
	rec = doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/"+secretCalendarID, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("a dm_only calendar must read as NotFound to an anonymous public visitor, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Secret Calendar") {
		t.Errorf("response must never contain the dm_only calendar's name: %s", rec.Body.String())
	}

	// Writes still bounce even on a public campaign.
	rec = doRequest(e, http.MethodPost, "/campaigns/camp-1/calendars", "")
	if rec.Code == http.StatusCreated {
		t.Error("anonymous visitor must not be able to create a calendar on a public campaign")
	}
	rec = doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/event-kinds", "")
	if rec.Code == http.StatusOK {
		t.Error("event kinds are Owner-only structure — must not be reachable at all on the public group (no route registered there)")
	}
}

func TestRouteGates_PublicCampaign_MemberStillGetsOwnRole(t *testing.T) {
	// A real member hitting the public-capable group's path (which wins the
	// registration, see routes.go) must still be resolved to their actual
	// role, not downgraded to the public/anonymous identity.
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner}
	e, svc := newAccessTestRouter(true, true, roles)

	rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/"+secretCalendarID, "u-owner")
	if rec.Code != http.StatusOK {
		t.Fatalf("the campaign's own Owner must be able to read the dm_only calendar even via the public group, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.lastViewer.Role() != int(permissions.RoleOwner) || svc.lastViewer.UserID() != "u-owner" {
		t.Errorf("member's real role must be used, got role=%d userID=%q", svc.lastViewer.Role(), svc.lastViewer.UserID())
	}
}

// --- Addon disabled ---

func TestRouteGates_AddonDisabled_AnswersLikeMapsDoes(t *testing.T) {
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner}
	e, _ := newAccessTestRouter(false, false, roles)

	// A fetch()-style JSON caller (Accept: application/json, exactly how
	// this API's real consumers call it) gets a 404 JSON refusal — the same
	// addons.RequireAddon branch maps.RequireAddon reuses for its own JSON
	// endpoints with the addon off (see addons/middleware.go).
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/calendars/cal-1", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "u-owner"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("a JSON/API request with the calendar addon disabled must get 404 (addons.RequireAddon's documented behavior), got %d: %s", rec.Code, rec.Body.String())
	}
}
