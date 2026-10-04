package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeCalendarSvcForAPI records what the handler passes to the calendar
// service. Only the methods the rebuilt routes call are implemented; the
// embedded interface panics on anything else, which is the point.
type fakeCalendarSvcForAPI struct {
	calendar.CalendarService

	cal    *calendar.Calendar
	events []calendar.Event

	defaultCampaign string
	viewers         []permissions.Viewer
	listArgs        [4]any // calendarID, campaignID, year, month
	getArgs         [3]string
	created         *calendar.CreateEventInput
	createArgs      [2]string
	updated         *calendar.UpdateEventInput
	updateArgs      [3]string
	deleteArgs      [3]string
	setDateArgs     []any
}

func (f *fakeCalendarSvcForAPI) GetPrimaryCalendarForViewer(_ context.Context, campaignID string, v permissions.Viewer) (*calendar.Calendar, error) {
	f.defaultCampaign = campaignID
	f.viewers = append(f.viewers, v)
	if f.cal == nil || f.cal.CampaignID != campaignID {
		return nil, apperror.NewNotFound("calendar not found")
	}
	cp := *f.cal
	return &cp, nil
}

func (f *fakeCalendarSvcForAPI) ListEventsForMonth(_ context.Context, calendarID, campaignID string, year, month int, _ permissions.Viewer) ([]calendar.Event, error) {
	f.listArgs = [4]any{calendarID, campaignID, year, month}
	return append([]calendar.Event(nil), f.events...), nil
}

func (f *fakeCalendarSvcForAPI) GetEventForViewer(_ context.Context, eventID, calendarID, campaignID string, _ permissions.Viewer) (*calendar.Event, error) {
	f.getArgs = [3]string{eventID, calendarID, campaignID}
	for _, e := range f.events {
		if e.ID == eventID {
			cp := e
			return &cp, nil
		}
	}
	return nil, apperror.NewNotFound("event not found")
}

func (f *fakeCalendarSvcForAPI) CreateEvent(_ context.Context, calendarID, campaignID string, in calendar.CreateEventInput) (*calendar.Event, error) {
	f.created, f.createArgs = &in, [2]string{calendarID, campaignID}
	return &calendar.Event{ID: "evt-new", CalendarID: calendarID, Name: in.Name, Visibility: in.Visibility}, nil
}

func (f *fakeCalendarSvcForAPI) UpdateEvent(_ context.Context, eventID, calendarID, campaignID string, in calendar.UpdateEventInput, _ permissions.Viewer) error {
	f.updated, f.updateArgs = &in, [3]string{eventID, calendarID, campaignID}
	return nil
}

func (f *fakeCalendarSvcForAPI) DeleteEvent(_ context.Context, eventID, calendarID, campaignID string, _ permissions.Viewer) error {
	f.deleteArgs = [3]string{eventID, calendarID, campaignID}
	return nil
}

func (f *fakeCalendarSvcForAPI) SetCurrentDate(_ context.Context, calendarID, campaignID string, year, month, day, hour, minute int) error {
	f.setDateArgs = []any{calendarID, campaignID, year, month, day, hour, minute}
	return nil
}

// stubCampaignSvcForCalendarAPI answers the membership lookups the handler's
// viewer and owner gate make.
type stubCampaignSvcForCalendarAPI struct {
	campaigns.CampaignService
	role      campaigns.Role
	granted   bool
	memberErr error
}

func (s *stubCampaignSvcForCalendarAPI) GetMember(context.Context, string, string) (*campaigns.CampaignMember, error) {
	if s.memberErr != nil {
		return nil, s.memberErr
	}
	return &campaigns.CampaignMember{Role: s.role}, nil
}

func (s *stubCampaignSvcForCalendarAPI) IsUserDmGranted(context.Context, string, string) (bool, error) {
	return s.granted, nil
}

func newCalendarFixture() *fakeCalendarSvcForAPI {
	return &fakeCalendarSvcForAPI{
		cal: &calendar.Calendar{
			ID: "cal-1", CampaignID: "camp-1", Mode: calendar.ModeFantasy, Name: "Harptos",
			CurrentYear: 1492, CurrentMonth: 3, CurrentDay: 7, HoursPerDay: 24, MinutesPerHour: 60,
		},
		events: []calendar.Event{
			{ID: "evt-public", CalendarID: "cal-1", Name: "Fair", Year: 1492, Month: 3, Day: 9, Visibility: "everyone"},
			{ID: "evt-gm", CalendarID: "cal-1", Name: "Ambush", Year: 1492, Month: 3, Day: 9, Visibility: "dm_only"},
		},
	}
}

// bearerKey is a stored module key for camp-1; sessionKey is the session
// door's synthetic key for a signed-in member.
func bearerKey() *APIKey { return &APIKey{ID: 42, CampaignID: "camp-1", UserID: "user-gm"} }
func sessionKey() *APIKey {
	return &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "user-p"}
}

func callCalendarAPI(t *testing.T, h *CalendarAPIHandler, fn func(echo.Context) error, method, target, body string, key *APIKey, params ...string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	names, values := []string{"id"}, []string{"camp-1"}
	for i := 0; i+1 < len(params); i += 2 {
		names, values = append(names, params[i]), append(values, params[i+1])
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	if key != nil {
		c.Set(apiKeyContextKey, key)
	}
	return rec, fn(c)
}

func statusOf(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

// TestCalendarAPI_ViewerMatchesCalendarPages: the handler builds the same
// Viewer the calendar pages build (VisibilityRole), so the service filters a
// Foundry or session caller exactly as it filters the web.
func TestCalendarAPI_ViewerMatchesCalendarPages(t *testing.T) {
	cases := []struct {
		name     string
		key      *APIKey
		camp     *stubCampaignSvcForCalendarAPI
		wantRole int
		wantUser string
	}{
		{"module key, owner", bearerKey(), &stubCampaignSvcForCalendarAPI{role: campaigns.RoleOwner}, 3, "user-gm"},
		{"session player", sessionKey(), &stubCampaignSvcForCalendarAPI{role: campaigns.RolePlayer}, 1, "user-p"},
		{"session scribe", sessionKey(), &stubCampaignSvcForCalendarAPI{role: campaigns.RoleScribe}, 2, "user-p"},
		{"session player with co-DM grant", sessionKey(), &stubCampaignSvcForCalendarAPI{role: campaigns.RolePlayer, granted: true}, 3, "user-p"},
		{"member lookup fails closed", sessionKey(), &stubCampaignSvcForCalendarAPI{memberErr: errors.New("db down")}, 0, "user-p"},
		{"grant without membership stays closed", sessionKey(), &stubCampaignSvcForCalendarAPI{memberErr: errors.New("not a member"), granted: true}, 0, "user-p"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newCalendarFixture()
			h := NewCalendarAPIHandler(nil, svc, tc.camp)
			if _, err := callCalendarAPI(t, h, h.GetCalendar, http.MethodGet, "/", "", tc.key); err != nil {
				t.Fatalf("GetCalendar: %v", err)
			}
			v := svc.viewers[0]
			if v.Role() != tc.wantRole || v.UserID() != tc.wantUser || v.IsSystem() {
				t.Errorf("viewer = role %d user %q system %v, want role %d user %q, never system",
					v.Role(), v.UserID(), v.IsSystem(), tc.wantRole, tc.wantUser)
			}
		})
	}
}

// TestCalendarAPI_ScopedToPathCampaign: every service call carries the URL's
// campaign and the calendar resolved for it, never an id from the body or an
// event id alone. RequireCampaignMatch has already pinned the URL's campaign
// to the key's, so this is what keeps a sync from crossing campaigns.
func TestCalendarAPI_ScopedToPathCampaign(t *testing.T) {
	svc := newCalendarFixture()
	h := NewCalendarAPIHandler(nil, svc, &stubCampaignSvcForCalendarAPI{role: campaigns.RoleOwner})
	key := bearerKey()

	if _, err := callCalendarAPI(t, h, h.ListEvents, http.MethodGet, "/?year=1492&month=4", "", key); err != nil {
		t.Fatal(err)
	}
	if svc.listArgs != [4]any{"cal-1", "camp-1", 1492, 4} {
		t.Errorf("ListEventsForMonth args = %v", svc.listArgs)
	}
	if _, err := callCalendarAPI(t, h, h.GetEvent, http.MethodGet, "/", "", key, "eventID", "evt-public"); err != nil {
		t.Fatal(err)
	}
	if svc.getArgs != [3]string{"evt-public", "cal-1", "camp-1"} {
		t.Errorf("GetEventForViewer args = %v", svc.getArgs)
	}
	if _, err := callCalendarAPI(t, h, h.CreateEvent, http.MethodPost, "/", `{"name":"X","year":1,"month":1,"day":1,"calendar_id":"cal-other"}`, key); err != nil {
		t.Fatal(err)
	}
	if svc.createArgs != [2]string{"cal-1", "camp-1"} {
		t.Errorf("CreateEvent args = %v", svc.createArgs)
	}
	if _, err := callCalendarAPI(t, h, h.UpdateEvent, http.MethodPut, "/", `{"name":"Y"}`, key, "eventID", "evt-other-campaign"); err != nil {
		t.Fatal(err)
	}
	if svc.updateArgs != [3]string{"evt-other-campaign", "cal-1", "camp-1"} {
		t.Errorf("UpdateEvent args = %v", svc.updateArgs)
	}
	if _, err := callCalendarAPI(t, h, h.DeleteEvent, http.MethodDelete, "/", "", key, "eventID", "evt-public"); err != nil {
		t.Fatal(err)
	}
	if svc.deleteArgs != [3]string{"evt-public", "cal-1", "camp-1"} {
		t.Errorf("DeleteEvent args = %v", svc.deleteArgs)
	}
	if _, err := callCalendarAPI(t, h, h.SetDate, http.MethodPut, "/", `{"year":1493,"month":2,"day":3,"hour":4,"minute":5}`, key); err != nil {
		t.Fatal(err)
	}
	want := []any{"cal-1", "camp-1", 1493, 2, 3, 4, 5}
	for i := range want {
		if svc.setDateArgs[i] != want[i] {
			t.Errorf("SetCurrentDate args = %v, want %v", svc.setDateArgs, want)
			break
		}
	}

	// A campaign with no calendar the caller can see is the module's "no
	// calendar" 404, not someone else's calendar.
	svc.cal.CampaignID = "camp-other"
	if _, err := callCalendarAPI(t, h, h.GetCalendar, http.MethodGet, "/", "", key); statusOf(err) != http.StatusNotFound {
		t.Errorf("GetCalendar with no calendar in camp-1: err = %v, want 404", err)
	}
}

// TestCalendarAPI_GMOnlyVisibilityOnTheWire: stored dm_only goes out as
// gm-only (the module shows anything else to players), and gm-only coming
// in is stored as dm_only. An absent visibility on update stays absent.
func TestCalendarAPI_GMOnlyVisibilityOnTheWire(t *testing.T) {
	svc := newCalendarFixture()
	h := NewCalendarAPIHandler(nil, svc, &stubCampaignSvcForCalendarAPI{role: campaigns.RoleOwner})
	key := bearerKey()

	rec, err := callCalendarAPI(t, h, h.ListEvents, http.MethodGet, "/", "", key)
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Total != 2 || list.Data[0]["visibility"] != "everyone" || list.Data[1]["visibility"] != "gm-only" {
		t.Errorf("list visibilities = %v / %v (total %d), want everyone / gm-only",
			list.Data[0]["visibility"], list.Data[1]["visibility"], list.Total)
	}
	if strings.Contains(rec.Body.String(), "dm_only") {
		t.Errorf("stored value leaked to the wire: %s", rec.Body.String())
	}

	rec, err = callCalendarAPI(t, h, h.GetEvent, http.MethodGet, "/", "", key, "eventID", "evt-gm")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), `"visibility":"gm-only"`) {
		t.Errorf("GetEvent body = %s, want gm-only", rec.Body.String())
	}

	for _, in := range []string{"gm-only", "gm_only"} {
		rec, err = callCalendarAPI(t, h, h.CreateEvent, http.MethodPost, "/", `{"name":"N","year":1,"month":1,"day":1,"visibility":"`+in+`"}`, key)
		if err != nil {
			t.Fatal(err)
		}
		if svc.created.Visibility != "dm_only" {
			t.Errorf("create %q stored as %q, want dm_only", in, svc.created.Visibility)
		}
		if !strings.Contains(rec.Body.String(), `"visibility":"gm-only"`) {
			t.Errorf("create response = %s, want gm-only", rec.Body.String())
		}
	}

	if _, err := callCalendarAPI(t, h, h.UpdateEvent, http.MethodPut, "/", `{"visibility":"gm-only"}`, key, "eventID", "evt-public"); err != nil {
		t.Fatal(err)
	}
	if got, ok := svc.updated.Visibility.Get(); !ok || got != "dm_only" {
		t.Errorf("update gm-only -> %q (present %v), want dm_only", got, ok)
	}
	if _, err := callCalendarAPI(t, h, h.UpdateEvent, http.MethodPut, "/", `{"name":"Renamed"}`, key, "eventID", "evt-public"); err != nil {
		t.Fatal(err)
	}
	if svc.updated.Visibility.Present() {
		t.Error("update without visibility sent a visibility change; absent must preserve")
	}
}

// TestCalendarAPI_DmOnlyAuthoringFollowsViewer: only an Owner or co-DM may
// author a gm-only event; the handler hands the service that answer from
// the caller's Viewer, never from the request.
func TestCalendarAPI_DmOnlyAuthoringFollowsViewer(t *testing.T) {
	for _, tc := range []struct {
		role    campaigns.Role
		granted bool
		want    bool
	}{
		{campaigns.RoleScribe, false, false},
		{campaigns.RoleScribe, true, true},
		{campaigns.RoleOwner, false, true},
	} {
		svc := newCalendarFixture()
		h := NewCalendarAPIHandler(nil, svc, &stubCampaignSvcForCalendarAPI{role: tc.role, granted: tc.granted})
		if _, err := callCalendarAPI(t, h, h.CreateEvent, http.MethodPost, "/", `{"name":"N","year":1,"month":1,"day":1,"can_author_dm_only":true}`, sessionKey()); err != nil {
			t.Fatal(err)
		}
		if svc.created.CanAuthorDmOnly != tc.want {
			t.Errorf("role %d granted %v: CanAuthorDmOnly = %v, want %v", tc.role, tc.granted, svc.created.CanAuthorDmOnly, tc.want)
		}
	}
}

// TestCalendarAPI_OwnerOnlyWrites: deleting an event and moving the date are
// Owner-only on the web, so a Scribe's write permission is not enough here.
func TestCalendarAPI_OwnerOnlyWrites(t *testing.T) {
	for _, role := range []campaigns.Role{campaigns.RolePlayer, campaigns.RoleScribe} {
		svc := newCalendarFixture()
		h := NewCalendarAPIHandler(nil, svc, &stubCampaignSvcForCalendarAPI{role: role, granted: true})
		if _, err := callCalendarAPI(t, h, h.DeleteEvent, http.MethodDelete, "/", "", sessionKey(), "eventID", "evt-public"); statusOf(err) != http.StatusForbidden {
			t.Errorf("role %d DeleteEvent: err = %v, want 403", role, err)
		}
		if _, err := callCalendarAPI(t, h, h.SetDate, http.MethodPut, "/", `{"year":1,"month":1,"day":1}`, sessionKey()); statusOf(err) != http.StatusForbidden {
			t.Errorf("role %d SetDate: err = %v, want 403", role, err)
		}
		if svc.deleteArgs != [3]string{} || svc.setDateArgs != nil {
			t.Errorf("role %d reached the service: delete %v setDate %v", role, svc.deleteArgs, svc.setDateArgs)
		}
	}
}

// TestCalendarAPI_ConfirmDateNeedsRealKey: the confirm is the module saying
// it applied a date; a member on the session door is never the module.
func TestCalendarAPI_ConfirmDateNeedsRealKey(t *testing.T) {
	repo := &mockSyncAPIRepo{}
	h := NewCalendarAPIHandler(NewSyncAPIService(repo), newCalendarFixture(), &stubCampaignSvcForCalendarAPI{role: campaigns.RoleOwner})
	body := `{"year":1492,"month":3,"day":7}`

	if _, err := callCalendarAPI(t, h, h.ConfirmDate, http.MethodPost, "/", body, sessionKey()); statusOf(err) != http.StatusForbidden {
		t.Errorf("session ConfirmDate: err = %v, want 403", err)
	}
	if len(repo.confirmBeaconCalls) != 0 {
		t.Fatalf("session caller wrote a confirm: %v", repo.confirmBeaconCalls)
	}
	rec, err := callCalendarAPI(t, h, h.ConfirmDate, http.MethodPost, "/", body, bearerKey())
	if err != nil || rec.Code != http.StatusNoContent {
		t.Fatalf("module ConfirmDate: code %d err %v, want 204", rec.Code, err)
	}
	if len(repo.confirmBeaconCalls) != 1 || repo.confirmBeaconCalls[0].campaignID != "camp-1" {
		t.Errorf("confirm calls = %+v, want one for camp-1", repo.confirmBeaconCalls)
	}
}

// TestCalendarAPI_CurrentDateRealTimeSignal: tracks_real_time is the
// effective predicate the date push is refused on, so the module and
// SetCurrentDate never disagree.
func TestCalendarAPI_CurrentDateRealTimeSignal(t *testing.T) {
	zone := "Europe/London"
	for _, tc := range []struct {
		name   string
		mutate func(*calendar.Calendar)
		want   bool
	}{
		{"fantasy", func(*calendar.Calendar) {}, false},
		{"real life, tracking", func(c *calendar.Calendar) {
			c.Mode, c.TracksRealTime, c.RealTimeZone = calendar.ModeRealLife, true, &zone
		}, true},
		{"fantasy with a stray flag", func(c *calendar.Calendar) { c.TracksRealTime = true }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newCalendarFixture()
			tc.mutate(svc.cal)
			h := NewCalendarAPIHandler(nil, svc, &stubCampaignSvcForCalendarAPI{role: campaigns.RoleOwner})
			rec, err := callCalendarAPI(t, h, h.GetCurrentDate, http.MethodGet, "/", "", sessionKey())
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["tracks_real_time"] != tc.want || body["year"] != float64(1492) || body["day"] != float64(7) {
				t.Errorf("body = %v, want tracks_real_time %v on 1492-3-7", body, tc.want)
			}
		})
	}
}

// TestCalendarAPI_SetDateNamesTheDates: the sync history says which date a
// push asked for and which it would replace, for a refused push too.
func TestCalendarAPI_SetDateNamesTheDates(t *testing.T) {
	cases := []struct {
		name     string
		role     campaigns.Role
		wantCode int
	}{
		{"owner", campaigns.RoleOwner, 0},
		{"player refused", campaigns.RolePlayer, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newCalendarFixture()
			svc.cal.Months = []calendar.Month{{Name: "Hammer"}, {Name: "Alturiak"}, {Name: "Ches"}, {Name: "Tarsakh"}}
			h := NewCalendarAPIHandler(nil, svc, &stubCampaignSvcForCalendarAPI{role: tc.role})
			req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"year":1492,"month":4,"day":1}`))
			req.Header.Set("Content-Type", "application/json")
			c := echo.New().NewContext(req, httptest.NewRecorder())
			c.SetParamNames("id")
			c.SetParamValues("camp-1")
			c.Set(apiKeyContextKey, bearerKey())
			err := h.SetDate(c)
			if statusOf(err) != tc.wantCode {
				t.Fatalf("err = %v, want code %d", err, tc.wantCode)
			}
			res, ok := c.Get(historyResourceKey).(historyResource)
			if !ok || res.name != "Tarsakh 1, 1492" || res.was != "Ches 7, 1492" {
				t.Fatalf("history names %+v", res)
			}
		})
	}
}

// TestCalendarAPI_AudiencePlayersNarrows: `?audience=players` reads as an
// anonymous Player whatever the key's role, so the module's player-facing
// date bar gets Chronicle's own player filtering; without it the key's
// viewer is unchanged.
func TestCalendarAPI_AudiencePlayersNarrows(t *testing.T) {
	cases := []struct {
		name     string
		target   string
		wantRole int
		wantUser string
	}{
		{"players audience", "/?audience=players", int(campaigns.RolePlayer), ""},
		{"no audience", "/", int(campaigns.RoleOwner), "user-p"},
		{"unknown audience", "/?audience=gm", int(campaigns.RoleOwner), "user-p"},
	}
	routes := []struct {
		name string
		fn   func(*CalendarAPIHandler) func(echo.Context) error
	}{
		{"GetCalendar", func(h *CalendarAPIHandler) func(echo.Context) error { return h.GetCalendar }},
		{"GetCurrentDate", func(h *CalendarAPIHandler) func(echo.Context) error { return h.GetCurrentDate }},
		{"ListEvents", func(h *CalendarAPIHandler) func(echo.Context) error { return h.ListEvents }},
	}
	for _, r := range routes {
		for _, tc := range cases {
			t.Run(r.name+"/"+tc.name, func(t *testing.T) {
				svc := newCalendarFixture()
				h := NewCalendarAPIHandler(nil, svc, &stubCampaignSvcForCalendarAPI{role: campaigns.RoleOwner})
				if _, err := callCalendarAPI(t, h, r.fn(h), http.MethodGet, tc.target, "", sessionKey()); err != nil {
					t.Fatalf("%s: %v", r.name, err)
				}
				v := svc.viewers[0]
				if v.Role() != tc.wantRole || v.UserID() != tc.wantUser || v.IsSystem() || v.SkipsPerUserRules() != (tc.wantRole >= int(campaigns.RoleOwner)) {
					t.Errorf("viewer = role %d user %q system %v, want role %d user %q",
						v.Role(), v.UserID(), v.IsSystem(), tc.wantRole, tc.wantUser)
				}
			})
		}
	}
}
