package syncapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

type fakeRateLimitRaiser struct {
	from, to int
	before   time.Time
	n        int64
	err      error
}

func (f *fakeRateLimitRaiser) RaiseKeyRateLimits(_ context.Context, from, to int, createdBefore time.Time) (int64, error) {
	f.from, f.to, f.before = from, to, createdBefore
	return f.n, f.err
}

func TestReconcileLegacyRateLimits(t *testing.T) {
	tests := []struct {
		name    string
		raiser  *fakeRateLimitRaiser
		want    int64
		wantErr bool
	}{
		{name: "raises old-default keys", raiser: &fakeRateLimitRaiser{n: 3}, want: 3},
		{name: "nothing to raise", raiser: &fakeRateLimitRaiser{}, want: 0},
		{name: "store error", raiser: &fakeRateLimitRaiser{err: errors.New("db down")}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReconcileLegacyRateLimits(context.Background(), tt.raiser)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("changed = %d, want %d", got, tt.want)
			}
			// Only keys on exactly the old default, made before the cutoff, move.
			if tt.raiser.from != 60 || tt.raiser.to != DefaultKeyRateLimit || !tt.raiser.before.Equal(legacyDefaultCutoff) {
				t.Errorf("raised %d→%d before %v", tt.raiser.from, tt.raiser.to, tt.raiser.before)
			}
		})
	}
	if _, err := ReconcileLegacyRateLimits(context.Background(), nil); err == nil {
		t.Error("nil store: want error")
	}
}

func TestRetryAfterSeconds(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		resetAt time.Time
		want    int
	}{
		{name: "rounds up", resetAt: now.Add(12300 * time.Millisecond), want: 13},
		{name: "whole minute", resetAt: now.Add(time.Minute), want: 60},
		{name: "already reset", resetAt: now.Add(-time.Second), want: 1},
		{name: "under a second", resetAt: now.Add(200 * time.Millisecond), want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryAfterSeconds(tt.resetAt, now); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

// A refused request names the real wait, not a flat minute, so a client
// that honours it resumes as soon as the window reopens.
func TestRateLimit_RetryAfterIsTheWindowsRemainder(t *testing.T) {
	e := echo.New()
	key := &APIKey{ID: 424242, CampaignID: "camp-1", RateLimit: 2}
	h := RateLimit(NewSyncAPIService(&mockSyncAPIRepo{}))(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	var lastErr error
	var rec *httptest.ResponseRecorder
	for i := 0; i < 3; i++ {
		rec = httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
		c.Set(apiKeyContextKey, key)
		lastErr = h(c)
	}
	var appErr *apperror.AppError
	if !errors.As(lastErr, &appErr) || appErr.Code != http.StatusTooManyRequests {
		t.Fatalf("third request: err = %v, want 429", lastErr)
	}
	secs, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	if err != nil || secs < 1 || secs > 60 {
		t.Errorf("Retry-After = %q, want 1..60 seconds", rec.Header().Get("Retry-After"))
	}
}
