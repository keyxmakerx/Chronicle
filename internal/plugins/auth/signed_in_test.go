package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	uaFirefoxLinux = "Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0"
	uaSafariIPad   = "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
)

func codeOf(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

// The list holds only the person's own browsers, marks the one asking, and
// signing one out works only on the person's own, never on this browser.
func TestMySessions_OwnOnly(t *testing.T) {
	svc, mr := newTestAuthServiceWithRedis(t, &mockUserRepo{})
	ctx := context.Background()
	alice, bob := &User{ID: "alice"}, &User{ID: "bob"}

	here, err := svc.createSession(ctx, alice, "", uaFirefoxLinux)
	if err != nil {
		t.Fatal(err)
	}
	ipad, _ := svc.createSession(ctx, alice, "", uaSafariIPad)
	gone, _ := svc.createSession(ctx, alice, "", uaFirefoxLinux)
	mr.Del(sessionKeyPrefix + gone) // expired, still in the set
	bobs, _ := svc.createSession(ctx, bob, "", uaFirefoxLinux)

	list, err := svc.ListMySessions(ctx, "alice", here)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("listed %d sessions, want this browser and the iPad: %+v", len(list), list)
	}
	if !list[0].Current || list[0].Device != "Firefox on Linux" || list[0].Mobile {
		t.Errorf("first row %+v, want this browser, Firefox on Linux", list[0])
	}
	if list[1].Current || list[1].Device != "Safari on iPad" || !list[1].Mobile {
		t.Errorf("second row %+v, want the iPad", list[1])
	}
	for _, s := range list {
		if s.ID == here || s.ID == ipad || len(s.ID) != 64 {
			t.Errorf("row ID %q must be the token's hash, never the token", s.ID)
		}
	}
	if mr.Exists(sessionKeyPrefix+gone) || isMember(t, mr, "alice", gone) {
		t.Error("an expired session's token stayed in the user's set")
	}

	if err := svc.SignOutMySession(ctx, "alice", hashToken(bobs), here); codeOf(err) != http.StatusNotFound {
		t.Errorf("signing out someone else's session: %v, want NotFound", err)
	}
	if !mr.Exists(sessionKeyPrefix + bobs) {
		t.Fatal("another person's session was ended")
	}
	if err := svc.SignOutMySession(ctx, "alice", hashToken(here), here); codeOf(err) != http.StatusBadRequest {
		t.Errorf("signing out this browser from the list: %v, want BadRequest", err)
	}
	if err := svc.SignOutMySession(ctx, "alice", hashToken(ipad), here); err != nil {
		t.Fatalf("signing out the iPad: %v", err)
	}
	if mr.Exists(sessionKeyPrefix + ipad) {
		t.Error("the iPad's session survived Sign out")
	}
	if !mr.Exists(sessionKeyPrefix + here) {
		t.Error("signing out the iPad ended this browser too")
	}
}

func isMember(t *testing.T, mr interface {
	SIsMember(string, string) (bool, error)
}, userID, token string) bool {
	t.Helper()
	ok, _ := mr.SIsMember(userSessionsKeyPrefix+userID, token)
	return ok
}

func TestDeviceLabel(t *testing.T) {
	cases := []struct {
		ua, want string
		mobile   bool
	}{
		{uaFirefoxLinux, "Firefox on Linux", false},
		{uaSafariIPad, "Safari on iPad", true},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36 Edg/130.0", "Edge on Windows", false},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36", "Chrome on Mac", false},
		{"Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Mobile Safari/537.36", "Chrome on Android", true},
		{"curl/8.0", "Unknown browser", false},
		{"", "Unknown browser", false},
	}
	for _, tc := range cases {
		got, mobile := deviceLabel(tc.ua)
		if got != tc.want || mobile != tc.mobile {
			t.Errorf("deviceLabel(%q) = %q, %v; want %q, %v", tc.ua, got, mobile, tc.want, tc.mobile)
		}
	}
}

// fakeApps is an in-memory ConnectedApps.
type fakeApps struct {
	apps         []ConnectedApp
	disconnected []string
}

func (f *fakeApps) ListForUser(context.Context, string) ([]ConnectedApp, error) { return f.apps, nil }
func (f *fakeApps) DisconnectForUser(_ context.Context, _, id string) error {
	for i, a := range f.apps {
		if a.ID == id {
			f.apps = append(f.apps[:i], f.apps[i+1:]...)
			f.disconnected = append(f.disconnected, id)
			return nil
		}
	}
	return apperror.NewNotFound("app not found")
}

// Sign out everywhere ends every session, this one included, and the
// connected apps go with them through the sessions-revoked hook.
func TestSignOutEverywhere(t *testing.T) {
	svc, mr := newTestAuthServiceWithRedis(t, &mockUserRepo{})
	ctx := context.Background()
	revoked := ""
	svc.onSessionsRevoked = append(svc.onSessionsRevoked, func(_ context.Context, userID string) { revoked = userID })
	here, _ := svc.createSession(ctx, &User{ID: "alice"}, "", uaFirefoxLinux)
	other, _ := svc.createSession(ctx, &User{ID: "alice"}, "", uaSafariIPad)

	h := NewHandler(svc, time.Hour)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/account/signed-in/sign-out-everywhere", nil)
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: here})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyUserID, "alice")
	if err := h.SignOutEverywhereAPI(c); err != nil {
		t.Fatal(err)
	}
	if mr.Exists(sessionKeyPrefix+here) || mr.Exists(sessionKeyPrefix+other) {
		t.Error("a session survived Sign out everywhere")
	}
	if revoked != "alice" {
		t.Error("connected apps were not ended with the sessions")
	}
	if rec.Header().Get("HX-Redirect") != "/login" {
		t.Errorf("HX-Redirect %q, want /login", rec.Header().Get("HX-Redirect"))
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), sessionCookieName+"=;") {
		t.Errorf("session cookie not cleared: %q", rec.Header().Get("Set-Cookie"))
	}
}

// Disconnecting an app answers with the card without it; one already gone
// says so instead of failing.
func TestDisconnectAppAPI(t *testing.T) {
	svc, _ := newTestAuthServiceWithRedis(t, &mockUserRepo{})
	h := NewHandler(svc, time.Hour)
	apps := &fakeApps{apps: []ConnectedApp{{ID: "g1", Name: "Foundry (notes and calendar)", Place: "Therin", Origin: "https://vtt.example", LastUsed: time.Now()}}}
	h.SetConnectedApps(apps)
	e := echo.New()
	post := func(id string) string {
		req := httptest.NewRequest(http.MethodPost, "/account/signed-in/apps/"+id+"/disconnect", nil)
		req.Header.Set("HX-Request", "true")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(id)
		c.Set(contextKeyUserID, "alice")
		if err := h.DisconnectAppAPI(c); err != nil {
			t.Fatal(err)
		}
		return rec.Body.String()
	}
	body := post("g1")
	if len(apps.disconnected) != 1 || strings.Contains(body, "Therin") || !strings.Contains(body, "Disconnected.") {
		t.Errorf("after disconnecting, card still lists the app or says nothing:\n%s", body)
	}
	if body := post("g1"); !strings.Contains(body, "already disconnected") {
		t.Errorf("a second press should say it was already disconnected:\n%s", body)
	}
}
