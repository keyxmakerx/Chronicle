// logging_endtoend_test.go complements logging_test.go's unit coverage of
// redactQuery with a full drive through the REAL, exported RequestLogger()
// middleware (registered globally in internal/app/app.go, no path exclusion
// for /media), capturing what slog actually emits — pinning that a signed
// media request really flows through this logger and the credential really
// ends up redacted on the wire, not just inside the helper in isolation.
package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

// TestRequestLogger_RedactsSignedMediaURL_EndToEnd drives a request shaped
// exactly like a real signed media fetch — GET /media/<id>/thumb/300 with
// `expires` and `sig` query params, the query internal/plugins/media's
// URLSigner produces (signed_url.go) — through the real RequestLogger
// middleware, and inspects the actual log record it writes.
func TestRequestLogger_RedactsSignedMediaURL_EndToEnd(t *testing.T) {
	var buf bytes.Buffer
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prevLogger)

	e := echo.New()
	const liveSig = "deadbeefcafef00d0123456789abcdef"
	const liveExpires = "1234567890"
	req := httptest.NewRequest(http.MethodGet,
		"/media/file-1/thumb/300?expires="+liveExpires+"&sig="+liveSig, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := RequestLogger()(func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	if err := handler(c); err != nil {
		t.Fatalf("RequestLogger middleware chain returned error: %v", err)
	}

	logOutput := buf.String()
	t.Logf("captured log line: %s", strings.TrimSpace(logOutput))

	if strings.Contains(logOutput, liveSig) {
		t.Errorf("RequestLogger wrote the live signed-URL signature to the log in plaintext: %s", logOutput)
	}
	if strings.Contains(logOutput, liveExpires) {
		t.Errorf("RequestLogger wrote the live signed-URL expiry to the log in plaintext: %s", logOutput)
	}
	if !strings.Contains(logOutput, "REDACTED") {
		t.Errorf("expected the query field to show a REDACTED marker, got: %s", logOutput)
	}
}
