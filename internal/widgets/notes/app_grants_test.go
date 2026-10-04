package notes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeGrantRepo is an in-memory AppGrantRepository.
type fakeGrantRepo struct {
	byHash  map[string]*AppGrant
	touched int
}

func newFakeGrantRepo() *fakeGrantRepo { return &fakeGrantRepo{byHash: map[string]*AppGrant{}} }

func (r *fakeGrantRepo) Create(_ context.Context, g *AppGrant, hash string) error {
	cp := *g
	r.byHash[hash] = &cp
	return nil
}

func (r *fakeGrantRepo) FindByTokenHash(_ context.Context, hash string) (*AppGrant, error) {
	g, ok := r.byHash[hash]
	if !ok {
		return nil, apperror.NewNotFound("grant not found")
	}
	cp := *g
	return &cp, nil
}

func (r *fakeGrantRepo) ListLive(_ context.Context, campaignID, userID string) ([]AppGrant, error) {
	var out []AppGrant
	for _, g := range r.byHash {
		if g.CampaignID == campaignID && g.UserID == userID && g.RevokedAt == nil {
			out = append(out, *g)
		}
	}
	// Newest first, as the SQL orders it.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func (r *fakeGrantRepo) Revoke(_ context.Context, id string, at time.Time) error {
	for _, g := range r.byHash {
		if g.ID == id && g.RevokedAt == nil {
			t := at
			g.RevokedAt = &t
		}
	}
	return nil
}

func (r *fakeGrantRepo) Touch(_ context.Context, id string, at time.Time) error {
	for _, g := range r.byHash {
		if g.ID == id {
			t := at
			g.LastUsedAt = &t
			r.touched++
		}
	}
	return nil
}

func (r *fakeGrantRepo) RevokeAllForUser(_ context.Context, userID string, at time.Time) error {
	for _, g := range r.byHash {
		if g.UserID == userID && g.RevokedAt == nil {
			t := at
			g.RevokedAt = &t
		}
	}
	return nil
}

func newTestGrantService(now *time.Time) (*appGrantService, *fakeGrantRepo) {
	repo := newFakeGrantRepo()
	return &appGrantService{repo: repo, now: func() time.Time { return *now }}, repo
}

func TestAppGrant_IssueStoresOnlyTheHash(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	svc, repo := newTestGrantService(&now)
	token, g, err := svc.Issue(context.Background(), "camp", "user", "https://foundry.example")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, appGrantTokenPrefix) || len(token) < 40 {
		t.Fatalf("token %q is not a prefixed 256-bit token", token)
	}
	if _, ok := repo.byHash[token]; ok {
		t.Fatal("the raw token was stored")
	}
	if _, ok := repo.byHash[hashAppGrantToken(token)]; !ok {
		t.Fatal("the token's hash was not stored")
	}
	if g.Origin != "https://foundry.example" || g.UserID != "user" || g.CampaignID != "camp" {
		t.Fatalf("unexpected grant %+v", g)
	}
}

func TestAppGrant_Authenticate(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	svc, _ := newTestGrantService(&now)
	token, g, _ := svc.Issue(context.Background(), "camp", "user", "https://foundry.example")

	tests := []struct {
		name    string
		token   string
		setup   func()
		wantErr int
	}{
		{name: "valid", token: token},
		{name: "unknown token", token: appGrantTokenPrefix + "nope", wantErr: http.StatusUnauthorized},
		{name: "not a grant token", token: "chron_abcdef", wantErr: http.StatusUnauthorized},
		{name: "empty", token: "", wantErr: http.StatusUnauthorized},
		{name: "idle past the limit", token: token, setup: func() { now = now.Add(appGrantIdleLimit + time.Hour) }, wantErr: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			got, err := svc.Authenticate(context.Background(), tt.token)
			if tt.wantErr != 0 {
				ae, ok := err.(*apperror.AppError)
				if !ok || ae.Code != tt.wantErr {
					t.Fatalf("want %d, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil || got.ID != g.ID {
				t.Fatalf("want grant %s, got %v, %v", g.ID, got, err)
			}
		})
	}
}

func TestAppGrant_RevokedTokenStopsWorking(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	svc, _ := newTestGrantService(&now)
	token, g, _ := svc.Issue(context.Background(), "camp", "user", "https://foundry.example")

	// Another player cannot revoke it, nor can the owner from another campaign.
	if err := svc.Revoke(context.Background(), "camp", "someone-else", g.ID); err == nil {
		t.Fatal("another player revoked the grant")
	}
	if err := svc.Revoke(context.Background(), "other-camp", "user", g.ID); err == nil {
		t.Fatal("the grant was revoked through another campaign")
	}
	if _, err := svc.Authenticate(context.Background(), token); err != nil {
		t.Fatalf("grant should still work: %v", err)
	}

	if err := svc.Revoke(context.Background(), "camp", "user", g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(context.Background(), token); err == nil {
		t.Fatal("a revoked grant still authenticates")
	}
}

func TestAppGrant_TouchIsThrottled(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	svc, repo := newTestGrantService(&now)
	token, _, _ := svc.Issue(context.Background(), "camp", "user", "https://foundry.example")
	for i := 0; i < 5; i++ {
		if _, err := svc.Authenticate(context.Background(), token); err != nil {
			t.Fatal(err)
		}
	}
	if repo.touched != 1 {
		t.Fatalf("want 1 touch within the window, got %d", repo.touched)
	}
	now = now.Add(appGrantTouchEvery)
	_, _ = svc.Authenticate(context.Background(), token)
	if repo.touched != 2 {
		t.Fatalf("want a second touch after the window, got %d", repo.touched)
	}
}

func TestAppGrant_CapRevokesTheOldest(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	svc, _ := newTestGrantService(&now)
	first, _, _ := svc.Issue(context.Background(), "camp", "user", "https://foundry.example")
	for i := 0; i < maxAppGrantsPerUser; i++ {
		now = now.Add(time.Minute)
		if _, _, err := svc.Issue(context.Background(), "camp", "user", "https://foundry.example"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Authenticate(context.Background(), first); err == nil {
		t.Fatal("the oldest grant past the cap still works")
	}
	live, _ := svc.List(context.Background(), "camp", "user")
	if len(live) != maxAppGrantsPerUser {
		t.Fatalf("want %d live grants, got %d", maxAppGrantsPerUser, len(live))
	}
}

func TestNormalizeOrigin(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://foundry.example", "https://foundry.example"},
		{"https://Foundry.Example:30000", "https://foundry.example:30000"},
		{"https://foundry.example/", "https://foundry.example"},
		{"http://localhost:30000", "http://localhost:30000"},
		{"https://foundry.example/game", ""},
		{"https://foundry.example?x=1", ""},
		{"https://user:pw@foundry.example", ""},
		{"javascript:alert(1)", ""},
		{"foundry.example", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeOrigin(tt.in); got != tt.want {
			t.Errorf("normalizeOrigin(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// runGrantMiddleware runs RequireAppGrant for campaign "camp" with the given
// Authorization header and reports the status and the user the request ran as.
func runGrantMiddleware(t *testing.T, svc AppGrantService, authz string) (int, *auth.Session) {
	t.Helper()
	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.NoContent(ae.Code)
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	var seen *auth.Session
	e.GET("/api/notes-app/campaigns/:id/notes", func(c echo.Context) error {
		seen = auth.GetSession(c)
		return c.NoContent(http.StatusOK)
	}, RequireAppGrant(svc, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/notes-app/campaigns/camp/notes", nil)
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code, seen
}

func TestRequireAppGrant(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	svc, _ := newTestGrantService(&now)
	token, _, _ := svc.Issue(context.Background(), "camp", "user", "https://foundry.example")
	otherCampaign, _, _ := svc.Issue(context.Background(), "other-camp", "user", "https://foundry.example")

	tests := []struct {
		name     string
		authz    string
		wantCode int
	}{
		{"valid grant runs as the player", "Bearer " + token, http.StatusOK},
		{"no header", "", http.StatusUnauthorized},
		{"not bearer", "Basic " + token, http.StatusUnauthorized},
		{"grant for another campaign", "Bearer " + otherCampaign, http.StatusForbidden},
		{"garbage", "Bearer cnt_garbage", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, s := runGrantMiddleware(t, svc, tt.authz)
			if code != tt.wantCode {
				t.Fatalf("want %d, got %d", tt.wantCode, code)
			}
			if code != http.StatusOK {
				return
			}
			if s == nil || s.UserID != "user" {
				t.Fatalf("request did not run as the grant's player: %+v", s)
			}
			if s.IsAdmin {
				t.Fatal("a notes grant must never carry site admin")
			}
		})
	}
}

type staticOrigins map[string]bool

func (s staticOrigins) OriginAllowed(_ context.Context, o string) bool { return s[o] }

func (s staticOrigins) AllowedOrigins(context.Context) []string {
	var out []string
	for o := range s {
		out = append(out, o)
	}
	return out
}

func TestAllowRefusesAnOriginNotOnTheList(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	svc, repo := newTestGrantService(&now)
	h := NewAppGrantHandler(svc, staticOrigins{"https://foundry.example": true})

	tests := []struct {
		origin string
		ok     bool
	}{
		{"https://foundry.example", true},
		{"https://FOUNDRY.example/", true},
		{"https://evil.example", false},
		{"https://foundry.example.evil.example", false},
		{"", false},
	}
	for _, tt := range tests {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		c := e.NewContext(req, httptest.NewRecorder())
		_, ok := h.allowedOrigin(c, tt.origin)
		if ok != tt.ok {
			t.Errorf("origin %q allowed=%v, want %v", tt.origin, ok, tt.ok)
		}
	}
	if len(repo.byHash) != 0 {
		t.Fatal("checking an origin made a grant")
	}
}

func TestRequireAppGrant_ClosedGateRefuses(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	svc, _ := newTestGrantService(&now)
	token, _, _ := svc.Issue(context.Background(), "camp", "user", "https://foundry.example")

	for _, open := range []bool{true, false} {
		e := echo.New()
		e.HTTPErrorHandler = func(err error, c echo.Context) {
			if ae, ok := err.(*apperror.AppError); ok {
				_ = c.NoContent(ae.Code)
			}
		}
		gate := func(context.Context, string) (bool, error) { return open, nil }
		e.GET("/api/notes-app/campaigns/:id/notes", func(c echo.Context) error {
			return c.NoContent(http.StatusOK)
		}, RequireAppGrant(svc, gate))
		req := httptest.NewRequest(http.MethodGet, "/api/notes-app/campaigns/camp/notes", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		want := http.StatusOK
		if !open {
			want = http.StatusForbidden
		}
		if rec.Code != want {
			t.Errorf("gate open=%v: want %d, got %d", open, want, rec.Code)
		}
	}
}

func TestAppGrant_RevokeAllForUserEndsEveryCampaign(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	svc, _ := newTestGrantService(&now)
	a, _, _ := svc.Issue(context.Background(), "camp", "user", "https://foundry.example")
	b, _, _ := svc.Issue(context.Background(), "other-camp", "user", "https://foundry.example")
	other, _, _ := svc.Issue(context.Background(), "camp", "someone-else", "https://foundry.example")
	if err := svc.RevokeAllForUser(context.Background(), "user"); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{a, b} {
		if _, err := svc.Authenticate(context.Background(), tok); err == nil {
			t.Fatal("a grant outlived the user's sessions")
		}
	}
	if _, err := svc.Authenticate(context.Background(), other); err != nil {
		t.Fatalf("another player's grant was revoked: %v", err)
	}
}

func TestFrameAncestors(t *testing.T) {
	got := frameAncestors([]string{"https://foundry.example", "https://x.example/path", "javascript:x", "https://Other.Example:30000",
		"https://*", "https://a.example;x", "https://*.example", "http://[::1]:8080"})
	want := "frame-ancestors 'self' https://foundry.example https://other.example:30000 http://[::1]:8080"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestShowEmbed_FramableOnlyByAllowedOrigins(t *testing.T) {
	h := NewAppGrantHandler(nil, staticOrigins{"https://foundry.example": true})
	tests := []struct {
		name, path string
		wantCode   int
	}{
		{"journal", "/embed/campaigns/0b9f2c1e-1111-2222-3333-444455556666/notes/journal", http.StatusOK},
		{"jots", "/embed/campaigns/0b9f2c1e-1111-2222-3333-444455556666/notes/jots", http.StatusOK},
		{"calendar", "/embed/campaigns/0b9f2c1e-1111-2222-3333-444455556666/notes/calendar", http.StatusOK},
		{"unknown mode", "/embed/campaigns/0b9f2c1e-1111-2222-3333-444455556666/notes/admin", http.StatusNotFound},
		{"bad id", "/embed/campaigns/not-an-id!/notes/journal", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			e.HTTPErrorHandler = func(err error, c echo.Context) {
				if ae, ok := err.(*apperror.AppError); ok {
					_ = c.NoContent(ae.Code)
				}
			}
			// The real site-wide headers, so a change to their wording that
			// stops the swap shows up here.
			e.Use(middleware.SecurityHeaders())
			e.GET("/embed/campaigns/:id/notes/:mode", h.ShowEmbed)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.wantCode {
				t.Fatalf("want %d, got %d", tt.wantCode, rec.Code)
			}
			if rec.Code != http.StatusOK {
				return
			}
			csp := rec.Header().Get("Content-Security-Policy")
			if !strings.Contains(csp, "frame-ancestors 'self' https://foundry.example;") {
				t.Fatalf("frame-ancestors not set to the allowed origins: %q", csp)
			}
			if strings.Contains(csp, "'none'") || rec.Header().Get("X-Frame-Options") != "" {
				t.Fatalf("still unframeable: %q / %q", csp, rec.Header().Get("X-Frame-Options"))
			}
			if !strings.Contains(csp, "default-src 'self'") {
				t.Fatalf("the rest of the policy was lost: %q", csp)
			}
		})
	}
}

func TestAllow_ClosedGateMakesNoGrant(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	for _, open := range []bool{true, false} {
		svc, repo := newTestGrantService(&now)
		h := NewAppGrantHandler(svc, staticOrigins{"https://foundry.example": true})
		h.gate = func(context.Context, string) (bool, error) { return open, nil }

		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"origin":"https://foundry.example"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp"}})
		auth.SetSession(c, &auth.Session{UserID: "user"})

		err := h.Allow(c)
		if open {
			if err != nil || rec.Code != http.StatusCreated || len(repo.byHash) != 1 {
				t.Fatalf("open gate: err=%v code=%d grants=%d", err, rec.Code, len(repo.byHash))
			}
			continue
		}
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != http.StatusForbidden {
			t.Fatalf("closed gate: want 403, got %v", err)
		}
		if len(repo.byHash) != 0 {
			t.Fatal("closed gate still made a grant")
		}
	}
}

type fakeLinker struct {
	campaignID, userID string
	paths              []string
}

func (f *fakeLinker) SignedLinksForMember(_ context.Context, campaignID, userID string, paths []string) map[string]string {
	f.campaignID, f.userID, f.paths = campaignID, userID, paths
	return map[string]string{paths[0]: paths[0] + "?expires=1&sig=x"}
}

func TestMediaLinks_SignsAsTheGrantsPlayer(t *testing.T) {
	h := NewAppGrantHandler(nil, nil)
	l := &fakeLinker{}
	h.SetMediaLinker(l)

	many := make([]string, 150)
	for i := range many {
		many[i] = "/media/x"
	}
	body, _ := json.Marshal(map[string]any{"paths": many})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp"}})
	auth.SetSession(c, &auth.Session{UserID: "player"})

	if err := h.MediaLinks(c); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("err=%v code=%d", err, rec.Code)
	}
	if l.campaignID != "camp" || l.userID != "player" || len(l.paths) != maxMediaLinkPaths {
		t.Fatalf("signed for %q/%q with %d paths", l.campaignID, l.userID, len(l.paths))
	}
	if rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Body.String(), `"links"`) {
		t.Fatalf("headers %v body %s", rec.Header(), rec.Body.String())
	}
}

type recordingConnRevoker struct{ calls []string }

func (r *recordingConnRevoker) RevokeNotesAppClients(campaignID, userID string) {
	r.calls = append(r.calls, "notes:"+campaignID+":"+userID)
}

func (r *recordingConnRevoker) RevokeNotesAppClientsEverywhere(userID string) {
	r.calls = append(r.calls, "everywhere:"+userID)
}

func TestAppGrant_RevokeClosesOpenSockets(t *testing.T) {
	tests := []struct {
		name   string
		revoke func(svc *appGrantService, grantID string) error
		want   []string
	}{
		{"one grant drops the player's notes sockets in that campaign",
			func(svc *appGrantService, id string) error {
				return svc.Revoke(context.Background(), "camp", "user", id)
			},
			[]string{"notes:camp:user"}},
		{"someone else's grant drops nothing",
			func(svc *appGrantService, id string) error {
				return svc.Revoke(context.Background(), "camp", "intruder", id)
			},
			nil},
		{"all grants with the sessions drop the user's notes sockets everywhere",
			func(svc *appGrantService, _ string) error { return svc.RevokeAllForUser(context.Background(), "user") },
			[]string{"everywhere:user"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
			svc, _ := newTestGrantService(&now)
			rec := &recordingConnRevoker{}
			svc.SetConnectionRevoker(rec)
			_, g, err := svc.Issue(context.Background(), "camp", "user", "https://foundry.example")
			if err != nil {
				t.Fatal(err)
			}
			_ = tt.revoke(svc, g.ID)
			if strings.Join(rec.calls, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("socket drops %v, want %v", rec.calls, tt.want)
			}
		})
	}
}

func TestAppGrant_CapEvictionClosesOpenSockets(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	svc, _ := newTestGrantService(&now)
	rec := &recordingConnRevoker{}
	svc.SetConnectionRevoker(rec)
	for i := 0; i < maxAppGrantsPerUser; i++ {
		now = now.Add(time.Second)
		if _, _, err := svc.Issue(context.Background(), "camp", "user", "https://foundry.example"); err != nil {
			t.Fatal(err)
		}
	}
	if len(rec.calls) != 0 {
		t.Fatalf("sockets dropped before the cap was passed: %v", rec.calls)
	}
	now = now.Add(time.Second)
	if _, _, err := svc.Issue(context.Background(), "camp", "user", "https://foundry.example"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(rec.calls, ",") != "notes:camp:user" {
		t.Fatalf("socket drops %v after a grant was evicted", rec.calls)
	}
}
