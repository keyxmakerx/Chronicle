package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

const (
	// mediaUploadPath is the media upload route.
	mediaUploadPath = "/media/upload"

	// calendarImportBodyLimit is the fixed pre-CSRF cap for the calendar import
	// routes: their handlers allow 10 MB of file, plus form overhead.
	calendarImportBodyLimit = 11 << 20

	// systemZipBodyLimit is the pre-CSRF cap for a custom game system zip:
	// the handler's own limit plus 1 MB of form overhead.
	systemZipBodyLimit = systems.MaxSystemZipSize + 1<<20

	// defaultUploadCeiling applies when no upload size can be worked out at
	// all (the setting unreadable and the configured size 0).
	defaultUploadCeiling = 10 << 20

	// uploadCeilingTTL is how long the ceiling is reused. A changed setting
	// applies within this time, without a restart and without a database
	// read on every upload.
	uploadCeilingTTL = 10 * time.Second
)

// ceilingSource is the part of the settings service the ceiling needs.
type ceilingSource interface {
	MaxUploadCeiling(ctx context.Context, fallback int64) (int64, error)
}

// uploadCeiling returns a function giving the largest per-file upload size
// anyone could be allowed, plus 10% for multipart framing, cached for a
// short time. On a read error it keeps the last good value, or else the
// configured size, so a database hiccup neither blocks uploads nor lifts the
// cap.
func uploadCeiling(src ceilingSource, configured int64, ttl time.Duration) func() int64 {
	var (
		mu      sync.Mutex
		value   int64
		expires time.Time
	)
	if configured <= 0 {
		configured = defaultUploadCeiling
	}
	return func() int64 {
		mu.Lock()
		defer mu.Unlock()
		if time.Now().Before(expires) {
			return value
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		c, err := src.MaxUploadCeiling(ctx, configured)
		switch {
		case err != nil:
			slog.Warn("upload size ceiling: settings read failed", slog.Any("error", err))
			if value == 0 {
				value = configured + configured/10
			}
			// Retry soon rather than after a full TTL.
			expires = time.Now().Add(time.Second)
			return value
		case c <= 0:
			c = configured
		}
		value = c + c/10
		expires = time.Now().Add(ttl)
		return value
	}
}

// registerUploadCaps installs the size caps for the upload routes whose own
// limit is larger than the global 2 MB one. They must be registered before
// the CSRF middleware: CSRF parses the multipart body to find its token, and
// that happens before anyone is signed in.
func registerUploadCaps(e *echo.Echo, mediaCeiling func() int64) {
	e.Use(middleware.BodyLimitFunc(mediaCeiling, func(c *echo.Context) bool {
		return !strings.HasPrefix(c.Request().URL.Path, mediaUploadPath)
	}))
	e.Use(middleware.BodyLimitFunc(
		func() int64 { return calendarImportBodyLimit },
		func(c *echo.Context) bool { return !isCalendarImportPath(c.Request().URL.Path) },
	))
	e.Use(systemZipCap())
}

// systemZipPathPattern matches the custom game system upload and its preview,
// whose handler reads a zip of up to systems.MaxSystemZipSize.
var systemZipPathPattern = regexp.MustCompile(`^/campaigns/[^/]+/systems/(upload|preview)$`)

func isSystemZipPath(path string) bool { return systemZipPathPattern.MatchString(path) }

// systemZipCap caps the game system zip routes. A body declared over the cap
// is refused with the handler's own wording, so the upload form's message
// says how large a zip may be rather than a bare "Request Entity Too Large".
func systemZipCap() echo.MiddlewareFunc {
	capped := middleware.BodyLimitFunc(
		func() int64 { return systemZipBodyLimit },
		func(c *echo.Context) bool { return !isSystemZipPath(c.Request().URL.Path) },
	)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		h := capped(next)
		return func(c *echo.Context) error {
			if isSystemZipPath(c.Request().URL.Path) && c.Request().ContentLength > systemZipBodyLimit {
				return &apperror.AppError{
					Code:    http.StatusRequestEntityTooLarge,
					Type:    "too_large",
					Message: fmt.Sprintf("file exceeds maximum size of %d MB", systems.MaxSystemZipSize/(1<<20)),
				}
			}
			return h(c)
		}
	}
}
