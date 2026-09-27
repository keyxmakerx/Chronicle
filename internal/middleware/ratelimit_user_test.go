package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// runUserRateLimit drives one request through UserRateLimit(identity, max,
// window) and reports whether the terminal handler was reached, plus the
// error UserRateLimit itself returned (nil if it let the request through).
func runUserRateLimit(t *testing.T, mw echo.MiddlewareFunc) (reached bool, err error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := mw(func(c echo.Context) error {
		reached = true
		return nil
	})
	err = handler(c)
	return reached, err
}

// TestUserRateLimit_BlocksOverLimit pins the core budget: the (N+1)th request
// from the same identity within the window is refused with the app's normal
// 429 error type, not a bespoke body, so every caller (JSON API, HTMX toast)
// gets the shape they already handle.
func TestUserRateLimit_BlocksOverLimit(t *testing.T) {
	mw := UserRateLimit(func(echo.Context) string { return "user-1" }, 3, time.Minute)

	for i := 0; i < 3; i++ {
		reached, err := runUserRateLimit(t, mw)
		if err != nil {
			t.Fatalf("request %d: unexpected error %v", i+1, err)
		}
		if !reached {
			t.Fatalf("request %d: handler not reached", i+1)
		}
	}

	reached, err := runUserRateLimit(t, mw)
	if reached {
		t.Fatal("4th request: handler reached, want it blocked")
	}
	appErr, ok := err.(*apperror.AppError)
	if !ok {
		t.Fatalf("4th request: error = %#v, want *apperror.AppError", err)
	}
	if appErr.Code != http.StatusTooManyRequests {
		t.Errorf("4th request: code = %d, want %d", appErr.Code, http.StatusTooManyRequests)
	}
	if appErr.Type != "too_many_requests" {
		t.Errorf("4th request: type = %q, want %q", appErr.Type, "too_many_requests")
	}
}

// TestUserRateLimit_SeparatesIdentities confirms the budget is per-key: one
// user hitting their limit never throttles another.
func TestUserRateLimit_SeparatesIdentities(t *testing.T) {
	current := "user-a"
	mw := UserRateLimit(func(echo.Context) string { return current }, 1, time.Minute)

	if reached, err := runUserRateLimit(t, mw); err != nil || !reached {
		t.Fatalf("user-a request 1: reached=%v err=%v, want reached=true err=nil", reached, err)
	}
	if reached, _ := runUserRateLimit(t, mw); reached {
		t.Fatal("user-a request 2: should be over budget")
	}

	current = "user-b"
	if reached, err := runUserRateLimit(t, mw); err != nil || !reached {
		t.Fatalf("user-b request 1: reached=%v err=%v, want reached=true err=nil", reached, err)
	}
}

// TestUserRateLimit_SkipsEmptyIdentity confirms an identity func returning ""
// (e.g. no authenticated user behind this request) is never throttled here —
// that's authentication's job, not this middleware's.
func TestUserRateLimit_SkipsEmptyIdentity(t *testing.T) {
	mw := UserRateLimit(func(echo.Context) string { return "" }, 1, time.Minute)

	for i := 0; i < 5; i++ {
		reached, err := runUserRateLimit(t, mw)
		if err != nil || !reached {
			t.Fatalf("request %d: reached=%v err=%v, want reached=true err=nil", i+1, reached, err)
		}
	}
}
