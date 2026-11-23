// event_outcome_handler_test.go: the routes behind what became of events,
// through the real router: who may read the passed list and flip the
// Calendar settings switch, and how status and line bind on an update.
package calendar

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

var outcomeRoles = map[string]campaigns.Role{
	"u-owner":  campaigns.RoleOwner,
	"u-scribe": campaigns.RoleScribe,
	"u-player": campaigns.RolePlayer,
}

func TestListPassedEventsAPI(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		query    string
		wantCode int
		wantFrom DayDate
	}{
		{name: "owner", user: "u-owner", query: "?from=5-1-9&to=5-1-12", wantCode: http.StatusOK, wantFrom: DayDate{5, 1, 9}},
		{name: "a year before zero", user: "u-owner", query: "?from=-3-1-9&to=-3-1-12", wantCode: http.StatusOK, wantFrom: DayDate{-3, 1, 9}},
		{name: "a bad date", user: "u-owner", query: "?from=5-1&to=5-1-12", wantCode: http.StatusBadRequest},
		{name: "scribe is not asked", user: "u-scribe", query: "?from=5-1-9&to=5-1-12", wantCode: http.StatusForbidden},
		{name: "player is not asked", user: "u-player", query: "?from=5-1-9&to=5-1-12", wantCode: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, svc := newAccessTestRouter(false, true, outcomeRoles)
			rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/cal-1/events/passed"+tt.query, tt.user)
			if rec.Code != tt.wantCode {
				t.Fatalf("code = %d, want %d: %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantCode != http.StatusOK {
				return
			}
			if svc.passedFrom != tt.wantFrom {
				t.Errorf("from = %v, want %v", svc.passedFrom, tt.wantFrom)
			}
			if !strings.Contains(rec.Body.String(), `"events":[`) || !strings.Contains(rec.Body.String(), `"truncated":false`) {
				t.Errorf("body = %s", rec.Body.String())
			}
		})
	}
}

func doForm(e http.Handler, method, path, userID string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userID})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestSetAskWhenDayPassesAPI(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		form     url.Values
		wantCode int
		wantAsk  *bool
	}{
		{name: "turned on", user: "u-owner", form: url.Values{"ask_sent": {"1"}, "ask": {"on"}}, wantCode: http.StatusOK, wantAsk: ptr(true)},
		{name: "turned off: the checkbox sends nothing", user: "u-owner", form: url.Values{"ask_sent": {"1"}}, wantCode: http.StatusOK, wantAsk: ptr(false)},
		{name: "a request without the switch is refused", user: "u-owner", form: url.Values{}, wantCode: http.StatusBadRequest},
		{name: "scribe may not", user: "u-scribe", form: url.Values{"ask_sent": {"1"}}, wantCode: http.StatusForbidden},
		{name: "player may not", user: "u-player", form: url.Values{"ask_sent": {"1"}}, wantCode: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, svc := newAccessTestRouter(false, true, outcomeRoles)
			rec := doForm(e, http.MethodPut, "/campaigns/camp-1/calendars/cal-1/day-passes", tt.user, tt.form)
			if rec.Code != tt.wantCode {
				t.Fatalf("code = %d, want %d: %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantAsk == nil {
				if svc.askSet != nil {
					t.Error("a refused request must not save")
				}
				return
			}
			if svc.askSet == nil || *svc.askSet != *tt.wantAsk {
				t.Fatalf("saved %v, want %v", svc.askSet, *tt.wantAsk)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "Saved") || !strings.Contains(body, `role="switch"`) {
				t.Errorf("the switch must come back saying Saved: %s", body)
			}
			if strings.Contains(body, "checked") != *tt.wantAsk {
				t.Errorf("checked state does not match %v: %s", *tt.wantAsk, body)
			}
		})
	}
}

func TestUpdateEventAPIBindsOutcome(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantStatus  string
		statusSent  bool
		lineSent    bool
		lineCleared bool
	}{
		{name: "status and line", body: `{"status":"happened","outcome":"Reached the well"}`, wantStatus: "happened", statusSent: true, lineSent: true},
		{name: "a cleared line", body: `{"outcome":null}`, lineSent: true, lineCleared: true},
		{name: "neither sent", body: `{"name":"Arrival"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, svc := newAccessTestRouter(false, true, outcomeRoles)
			rec := doRequestWithBody(e, http.MethodPut, "/campaigns/camp-1/calendars/cal-1/events/evt-1", "u-owner", tt.body)
			if rec.Code >= 300 {
				t.Fatalf("code = %d: %s", rec.Code, rec.Body.String())
			}
			in := svc.lastUpdate
			if s, ok := in.Status.Get(); ok != tt.statusSent || s != tt.wantStatus {
				t.Errorf("status = %q (sent %v), want %q (sent %v)", s, ok, tt.wantStatus, tt.statusSent)
			}
			if in.Outcome.Present() != tt.lineSent || in.Outcome.IsNull() != tt.lineCleared {
				t.Errorf("outcome present %v null %v, want %v %v", in.Outcome.Present(), in.Outcome.IsNull(), tt.lineSent, tt.lineCleared)
			}
		})
	}
}
