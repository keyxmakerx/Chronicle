// login_redirect_test.go — destination preservation through the login flow
// (C-CALV4-RSVP-P8B stage 1, ruling [PB-6]).
//
// Two things are pinned here, and the second is a security property:
//
//  1. An unauthenticated browser navigation carries where it was going into
//     /login?redirect=…, and the login form carries that through the POST as a
//     hidden field — exactly as register.templ:86 already does — so a member
//     who clicks a deep link from a cold inbox lands on the page they were
//     asked to visit instead of /dashboard.
//  2. Every destination is passed through sanitizeRedirect at BOTH ends (the
//     middleware's way out and the handler's way back), so the protocol-relative
//     open-redirect vectors "//evil.example" and "/\evil.example" can never
//     become a Location header. Before this slice, Login validated with a bare
//     strings.HasPrefix(redir, "/"), which accepts both.
package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// redirectStubService satisfies AuthService by embedding it: only the two
// methods the login round trip touches are implemented, and any other call
// would nil-panic loudly rather than pass silently.
type redirectStubService struct {
	AuthService
	validSession bool
}

func (s redirectStubService) Login(_ context.Context, input LoginInput) (string, *User, error) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if bcrypt.CompareHashAndPassword(hash, []byte(input.Password)) != nil {
		return "", nil, apperror.NewUnauthorized("invalid email or password")
	}
	return "session-token", &User{ID: "u1", Email: input.Email}, nil
}

func (s redirectStubService) ValidateSession(context.Context, string) (*Session, error) {
	if s.validSession {
		return &Session{UserID: "u1"}, nil
	}
	return nil, apperror.NewUnauthorized("invalid email or password")
}

// hiddenRedirectRE extracts the value of the login form's hidden redirect field.
var hiddenRedirectRE = regexp.MustCompile(`name="redirect" value="([^"]*)"`)

// hiddenRedirectField renders the login form via the real GET /login handler
// and returns the hidden field's value (and the raw body, for absence checks).
func hiddenRedirectField(t *testing.T, rawQueryRedirect string) (string, string) {
	t.Helper()
	e := echo.New()
	target := "/login"
	if rawQueryRedirect != "" {
		target += "?redirect=" + url.QueryEscape(rawQueryRedirect)
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h := NewHandler(redirectStubService{}, time.Hour)
	if err := h.LoginForm(e.NewContext(req, rec)); err != nil {
		t.Fatalf("LoginForm: %v", err)
	}
	body := rec.Body.String()
	m := hiddenRedirectRE.FindStringSubmatch(body)
	if m == nil {
		return "", body
	}
	return m[1], body
}

// postLogin submits the login form with the given hidden redirect field and
// returns the destination the handler chose (Location or HX-Redirect).
func postLogin(t *testing.T, redirectField string, htmx bool) string {
	t.Helper()
	e := echo.New()
	form := url.Values{}
	form.Set("email", "alice@example.com")
	form.Set("password", "password123")
	form.Set("redirect", redirectField)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	h := NewHandler(redirectStubService{}, time.Hour)
	if err := h.Login(e.NewContext(req, rec)); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if htmx {
		return rec.Header().Get("HX-Redirect")
	}
	return rec.Header().Get("Location")
}

// TestLoginRedirect_RoundTripAndOpenRedirect is the table the ruling asks for:
// a legitimate campaign deep link survives the form POST, and both
// protocol-relative vectors are dead at both ends.
func TestLoginRedirect_RoundTripAndOpenRedirect(t *testing.T) {
	tests := []struct {
		name string
		// raw is what arrives on GET /login?redirect=…
		raw string
		// wantField is what the hidden form field must carry.
		wantField string
		// wantDest is where a successful POST must send the browser.
		wantDest string
	}{
		{
			name:      "legitimate campaign deep link round trips",
			raw:       "/campaigns/c-123/availability",
			wantField: "/campaigns/c-123/availability",
			wantDest:  "/campaigns/c-123/availability",
		},
		{
			name:      "protocol-relative //evil is rejected at both ends",
			raw:       "//evil.example/steal",
			wantField: "",
			wantDest:  "/dashboard",
		},
		{
			name:      "backslash-relative /\\evil is rejected at both ends",
			raw:       "/\\evil.example/steal",
			wantField: "",
			wantDest:  "/dashboard",
		},
		{
			name:      "absolute off-site URL is rejected",
			raw:       "https://evil.example/steal",
			wantField: "",
			wantDest:  "/dashboard",
		},
		{
			name:      "no redirect at all still lands on the dashboard",
			raw:       "",
			wantField: "",
			wantDest:  "/dashboard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, body := hiddenRedirectField(t, tt.raw)
			if field != tt.wantField {
				t.Errorf("hidden redirect field = %q, want %q", field, tt.wantField)
			}
			if tt.wantField == "" && strings.Contains(body, "evil.example") {
				t.Errorf("the rejected destination leaked into the login page body")
			}
			if got := postLogin(t, field, false); got != tt.wantDest {
				t.Errorf("Location after POST = %q, want %q", got, tt.wantDest)
			}
			if got := postLogin(t, field, true); got != tt.wantDest {
				t.Errorf("HX-Redirect after POST = %q, want %q", got, tt.wantDest)
			}
		})
	}
}

// TestLoginRedirect_HandlerSanitizesForgedField proves the handler's own end:
// a caller who skips the form and posts a hostile redirect field directly is
// still sent to the dashboard. This is the assertion that would fail if Login
// went back to strings.HasPrefix(redir, "/").
func TestLoginRedirect_HandlerSanitizesForgedField(t *testing.T) {
	for _, forged := range []string{"//evil.example", "/\\evil.example", "https://evil.example", "javascript:alert(1)"} {
		if got := postLogin(t, forged, false); got != "/dashboard" {
			t.Errorf("forged redirect %q produced Location %q, want /dashboard", forged, got)
		}
	}
}

// assertSameOrigin fails unless loc is a same-origin path: no scheme, no
// authority, and not one of the two protocol-relative forms a browser would
// resolve against another host.
func assertSameOrigin(t *testing.T, from, loc string) {
	t.Helper()
	if loc == "" {
		t.Fatalf("request URI %q produced no destination at all", from)
	}
	if strings.HasPrefix(loc, "//") || strings.HasPrefix(loc, "/\\") {
		t.Fatalf("request URI %q produced a protocol-relative destination %q", from, loc)
	}
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("destination %q from %q does not parse: %v", loc, from, err)
	}
	if u.Scheme != "" || u.Host != "" {
		t.Fatalf("request URI %q produced an off-site destination %q", from, loc)
	}
}

// TestRequireAuth_PreservesDestination pins the middleware's way out: the page
// the visitor asked for is carried into /login as a sanitized ?redirect=, for
// both the plain-browser 303 and the HTMX HX-Redirect branch. API requests and
// hostile request URIs are unchanged.
func TestRequireAuth_PreservesDestination(t *testing.T) {
	call := func(target string, htmx bool) (int, string) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		rec := httptest.NewRecorder()
		mw := RequireAuth(redirectStubService{})
		handler := mw(func(echo.Context) error { return nil })
		if err := handler(e.NewContext(req, rec)); err != nil {
			t.Fatalf("middleware: %v", err)
		}
		if htmx {
			return rec.Code, rec.Header().Get("HX-Redirect")
		}
		return rec.Code, rec.Header().Get("Location")
	}

	t.Run("browser 303 carries the destination", func(t *testing.T) {
		code, loc := call("/campaigns/c-123/availability", false)
		if code != http.StatusSeeOther {
			t.Errorf("status = %d, want 303", code)
		}
		if want := "/login?redirect=" + url.QueryEscape("/campaigns/c-123/availability"); loc != want {
			t.Errorf("Location = %q, want %q", loc, want)
		}
	})

	t.Run("HTMX branch carries the same destination", func(t *testing.T) {
		_, loc := call("/campaigns/c-123/availability", true)
		if want := "/login?redirect=" + url.QueryEscape("/campaigns/c-123/availability"); loc != want {
			t.Errorf("HX-Redirect = %q, want %q", loc, want)
		}
	})

	t.Run("a non-GET navigation is not replayed as a destination", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/campaigns/c-123/calendar/ask", nil)
		rec := httptest.NewRecorder()
		handler := RequireAuth(redirectStubService{})(func(echo.Context) error { return nil })
		if err := handler(e.NewContext(req, rec)); err != nil {
			t.Fatalf("middleware: %v", err)
		}
		if loc := rec.Header().Get("Location"); loc != "/login" {
			t.Errorf("Location = %q, want a bare /login for a non-GET", loc)
		}
	})

	t.Run("API requests are unchanged", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/api/campaigns", nil)
		rec := httptest.NewRecorder()
		handler := RequireAuth(redirectStubService{})(func(echo.Context) error { return nil })
		if err := handler(e.NewContext(req, rec)); err != nil {
			t.Fatalf("middleware: %v", err)
		}
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("API status = %d, want 401", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "" {
			t.Errorf("an API 401 must carry no Location, got %q", loc)
		}
	})

	// The end-to-end invariant, stated as the property rather than a substring:
	// whatever request URI arrives, the destination that survives the middleware
	// AND the subsequent form POST must still be same-origin. Go's request-line
	// parser already strips the authority from "//evil.example/steal", and
	// sanitizeRedirect kills a literal "/\"; anything that slips past both (e.g.
	// the percent-escaped "/%5C…", which a browser resolves as an ordinary
	// same-origin path) must still be provably harmless.
	t.Run("a hostile request URI never yields an off-site destination", func(t *testing.T) {
		for _, target := range []string{"//evil.example/steal", "/\\evil.example", "/campaigns/c-1/availability"} {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()
			handler := RequireAuth(redirectStubService{})(func(echo.Context) error { return nil })
			if err := handler(e.NewContext(req, rec)); err != nil {
				t.Fatalf("middleware: %v", err)
			}
			loc := rec.Header().Get("Location")
			assertSameOrigin(t, target, loc)

			// Now walk the destination through the form POST and check the far end.
			u, err := url.Parse(loc)
			if err != nil {
				t.Fatalf("login target %q does not parse: %v", loc, err)
			}
			assertSameOrigin(t, target, postLogin(t, u.Query().Get("redirect"), false))
		}
	})
}
