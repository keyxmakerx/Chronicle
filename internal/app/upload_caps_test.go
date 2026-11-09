package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/config"
)

// passedCap stands for "got through the size caps and was refused by CSRF".
// The bare test server has no error handler for CSRF's error, so the exact
// code is not the point; 413 and a handler 200 are the outcomes that matter.
const passedCap = -1

func outcome(code int) int {
	if code == http.StatusRequestEntityTooLarge || code == http.StatusOK {
		return code
	}
	return passedCap
}

// newUploadCapApp builds the real middleware chain with a settable media
// ceiling. Requests carry no CSRF token, so anything that gets past the size
// caps is refused by CSRF: the tests read 413 as "stopped by the cap" and
// any other refusal as "got as far as CSRF".
func newUploadCapApp(ceiling *int64) *App {
	a := &App{
		Config:          &config.Config{},
		Echo:            echo.New(),
		UploadCeilingFn: func() int64 { return *ceiling },
	}
	a.setupMiddleware()
	ok := func(c *echo.Context) error { return c.NoContent(http.StatusOK) }
	a.Echo.POST("/media/upload", ok)
	a.Echo.POST("/campaigns/:id/calendars/wizard/import/preview", ok)
	a.Echo.POST("/campaigns/:id/calendars/other", ok)
	return a
}

func multipartRequest(path string, body *countingReader, size int64) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, body)
	req.ContentLength = size
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	return req
}

func TestUploadCapsRunBeforeCSRF(t *testing.T) {
	const mb = 1 << 20
	ceiling := int64(5 * mb)
	a := newUploadCapApp(&ceiling)

	tests := []struct {
		name       string
		path       string
		size       int64
		wantStatus int
	}{
		{"media upload under the ceiling reaches CSRF", "/media/upload", 4 * mb, passedCap},
		{"media upload over the ceiling is refused up front", "/media/upload", 6 * mb, http.StatusRequestEntityTooLarge},
		{"calendar import under its cap reaches CSRF", "/campaigns/c1/calendars/wizard/import/preview", 10*mb + mb/2, passedCap},
		{"calendar import over its cap is refused up front", "/campaigns/c1/calendars/wizard/import/preview", 12 * mb, http.StatusRequestEntityTooLarge},
		{"another calendar route keeps the global 2 MB cap", "/campaigns/c1/calendars/other", 3 * mb, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cr := &countingReader{r: strings.NewReader(strings.Repeat("a", int(tc.size)))}
			rec := httptest.NewRecorder()
			a.Echo.ServeHTTP(rec, multipartRequest(tc.path, cr, tc.size))
			if got := outcome(rec.Code); got != tc.wantStatus {
				t.Errorf("outcome = %d (status %d), want %d", got, rec.Code, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusRequestEntityTooLarge && cr.n != 0 {
				t.Errorf("server read %d bytes of a refused body, want none", cr.n)
			}
		})
	}

	// With no declared length the check at the door can't help, so the reader
	// itself must stop at the cap before CSRF's parse reads the rest.
	for _, tc := range []struct {
		name string
		path string
		cap  int64
	}{
		{"unsized media body is cut off at the ceiling", "/media/upload", 5 * mb},
		{"unsized calendar import body is cut off at its cap", "/campaigns/c1/calendars/wizard/import/preview", 11 * mb},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cr := &countingReader{r: strings.NewReader(strings.Repeat("a", int(tc.cap)*2))}
			a.Echo.ServeHTTP(httptest.NewRecorder(), multipartRequest(tc.path, cr, -1))
			if cr.n > tc.cap+64*1024 {
				t.Errorf("server read %d bytes, want about %d at most", cr.n, tc.cap)
			}
		})
	}
}

func TestMediaCeilingFollowsTheSetting(t *testing.T) {
	const mb = 1 << 20
	ceiling := int64(5 * mb)
	a := newUploadCapApp(&ceiling)

	status := func(size int64) int {
		cr := &countingReader{r: strings.NewReader(strings.Repeat("a", int(size)))}
		rec := httptest.NewRecorder()
		a.Echo.ServeHTTP(rec, multipartRequest("/media/upload", cr, size))
		return rec.Code
	}

	if got := status(8 * mb); got != http.StatusRequestEntityTooLarge {
		t.Fatalf("8 MB against a 5 MB ceiling: status %d, want 413", got)
	}
	ceiling = 20 * mb // The admin raises the limit; no restart.
	if got := outcome(status(8 * mb)); got != passedCap {
		t.Errorf("8 MB after raising the ceiling: status %d, want past the cap (stopped by CSRF)", got)
	}
	ceiling = 2 * mb // And lowers it again.
	if got := status(8 * mb); got != http.StatusRequestEntityTooLarge {
		t.Errorf("8 MB after lowering the ceiling: status %d, want 413", got)
	}
}

type fakeCeilingSource struct {
	calls int
	value int64
	err   error
	got   int64 // The fallback it was given.
}

func (f *fakeCeilingSource) MaxUploadCeiling(_ context.Context, fallback int64) (int64, error) {
	f.calls++
	f.got = fallback
	return f.value, f.err
}

func TestUploadCeilingFunc(t *testing.T) {
	t.Run("adds 10 percent and caches", func(t *testing.T) {
		src := &fakeCeilingSource{value: 100}
		fn := uploadCeiling(src, 50, time.Hour)
		if got := fn(); got != 110 {
			t.Errorf("ceiling = %d, want 110", got)
		}
		src.value = 1000
		if got := fn(); got != 110 || src.calls != 1 {
			t.Errorf("ceiling = %d after %d reads, want the cached 110 after 1", got, src.calls)
		}
	})
	t.Run("a changed setting shows once the cache expires", func(t *testing.T) {
		src := &fakeCeilingSource{value: 100}
		fn := uploadCeiling(src, 50, time.Nanosecond)
		fn()
		src.value = 200
		time.Sleep(time.Millisecond)
		if got := fn(); got != 220 {
			t.Errorf("ceiling = %d, want 220", got)
		}
	})
	t.Run("a read error falls back to the configured size", func(t *testing.T) {
		src := &fakeCeilingSource{err: errors.New("db down")}
		fn := uploadCeiling(src, 100, time.Hour)
		if got := fn(); got != 110 {
			t.Errorf("ceiling = %d, want 110", got)
		}
	})
	t.Run("a read error keeps the last good value", func(t *testing.T) {
		src := &fakeCeilingSource{value: 300}
		fn := uploadCeiling(src, 100, time.Nanosecond)
		fn()
		src.err = errors.New("db down")
		time.Sleep(time.Millisecond)
		if got := fn(); got != 330 {
			t.Errorf("ceiling = %d, want 330", got)
		}
	})
	t.Run("an unusable configured size still gets a cap", func(t *testing.T) {
		src := &fakeCeilingSource{err: errors.New("db down")}
		fn := uploadCeiling(src, 0, time.Hour)
		if got := fn(); got <= 0 {
			t.Errorf("ceiling = %d, want a positive cap", got)
		}
		if src.got != defaultUploadCeiling {
			t.Errorf("fallback passed on = %d, want %d", src.got, defaultUploadCeiling)
		}
	})
}
