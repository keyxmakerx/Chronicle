// Tests for the RequireJSONContentType middleware introduced by
// C-SEC-CHUNK-3-AMENDED (operator decision D-C3.1 — sub-group skip).
//
// The middleware sits on the /api/v1/* JSON group and rejects state-
// changing requests whose Content-Type is anything other than
// application/json. The lone multipart endpoint
// (POST /api/v1/campaigns/:id/media) re-mounts on a parallel sub-group
// that omits this middleware — see RegisterAPIRoutes.
package syncapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// newJSONContentTypeFixture returns an Echo instance with
// RequireJSONContentType mounted on a /test group plus a terminal
// handler that returns 200 OK. The same AppError-aware error handler
// used by the production stack maps the middleware's 415 to a JSON
// response with the expected status code.
func newJSONContentTypeFixture(t *testing.T) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		type errBody struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if appErr, ok := err.(*apperror.AppError); ok {
			_ = c.JSON(appErr.Code, errBody{
				Error:   appErr.Type,
				Message: appErr.Message,
			})
			return
		}
		_ = c.JSON(http.StatusInternalServerError, errBody{Error: "internal_error"})
	}
	g := e.Group("/test", RequireJSONContentType())
	pong := func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
	}
	g.POST("/echo", pong)
	g.PUT("/echo", pong)
	g.PATCH("/echo", pong)
	g.GET("/echo", pong)
	g.DELETE("/echo", pong)
	g.HEAD("/echo", pong)
	g.OPTIONS("/echo", pong)
	return e
}

// TestRequireJSONContentType_AcceptsApplicationJSON pins the positive
// path — a POST with Content-Type: application/json passes through.
func TestRequireJSONContentType_AcceptsApplicationJSON(t *testing.T) {
	e := newJSONContentTypeFixture(t)

	req := httptest.NewRequest(http.MethodPost, "/test/echo", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST application/json: status = %d, want 200; body = %s",
			rec.Code, rec.Body.String())
	}
}

// TestRequireJSONContentType_AcceptsApplicationJSONWithCharset pins
// the common "; charset=utf-8" suffix — must pass through, otherwise
// every JSON client that follows RFC 8259's charset advice breaks.
func TestRequireJSONContentType_AcceptsApplicationJSONWithCharset(t *testing.T) {
	e := newJSONContentTypeFixture(t)

	req := httptest.NewRequest(http.MethodPost, "/test/echo", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST application/json; charset=utf-8: status = %d, want 200; body = %s",
			rec.Code, rec.Body.String())
	}
}

// TestRequireJSONContentType_Rejects415 is the load-bearing negative
// path. A POST with text/html (a malicious proxy substituting an HTML
// error page, or a buggy client) must hit 415 BEFORE the handler
// runs — otherwise Echo's json.Unmarshal silently produces a zero-
// value struct and the handler acts on it. Drives the rejection
// against every state-changing method this middleware guards.
func TestRequireJSONContentType_Rejects415(t *testing.T) {
	cases := []struct {
		name        string
		method      string
		contentType string
	}{
		{"POST text/html", http.MethodPost, "text/html"},
		{"POST multipart/form-data", http.MethodPost, "multipart/form-data; boundary=xyz"},
		{"POST application/xml", http.MethodPost, "application/xml"},
		{"POST application/x-www-form-urlencoded", http.MethodPost, "application/x-www-form-urlencoded"},
		{"POST empty Content-Type", http.MethodPost, ""},
		{"PUT text/plain", http.MethodPut, "text/plain"},
		{"PATCH text/html", http.MethodPatch, "text/html"},
	}
	e := newJSONContentTypeFixture(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/test/echo", strings.NewReader(""))
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnsupportedMediaType {
				t.Fatalf("%s: status = %d, want 415; body = %s",
					tc.name, rec.Code, rec.Body.String())
			}
			// Structured error body so a client surface can render it
			// without resorting to bare status codes.
			body := rec.Body.String()
			if !strings.Contains(body, "unsupported_media_type") {
				t.Errorf("%s: body missing unsupported_media_type type: %s",
					tc.name, body)
			}
		})
	}
}

// TestRequireJSONContentType_SafeMethodsPassThrough confirms GET /
// DELETE / HEAD / OPTIONS skip the Content-Type check — they carry
// no request body so a Content-Type lock would be meaningless and
// would break callers that omit the header on safe methods (which
// is RFC-permitted).
func TestRequireJSONContentType_SafeMethodsPassThrough(t *testing.T) {
	e := newJSONContentTypeFixture(t)
	for _, method := range []string{http.MethodGet, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/test/echo", nil)
			// No Content-Type set on purpose.
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s with no Content-Type: status = %d, want 200; body = %s",
					method, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestRequireJSONContentType_SubGroupSkipPattern pins the wiring
// invariant operator decision D-C3.1 requires: a sub-group that
// re-uses /test as its prefix but DOES NOT include
// RequireJSONContentType() must accept multipart/form-data. This is
// the structural equivalent of v1Multipart in RegisterAPIRoutes — if
// someone accidentally also gates the multipart sub-group with this
// middleware, UploadMedia returns 415 in production. This test
// asserts the negative-by-construction.
func TestRequireJSONContentType_SubGroupSkipPattern(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if appErr, ok := err.(*apperror.AppError); ok {
			_ = c.JSON(appErr.Code, map[string]string{"type": appErr.Type})
			return
		}
		_ = c.JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
	}
	// JSON-locked group — mirrors v1.
	v1 := e.Group("/test", RequireJSONContentType())
	v1.POST("/json", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	// Multipart sub-group at the same prefix — mirrors v1Multipart.
	// NO RequireJSONContentType() here.
	v1Multipart := e.Group("/test")
	v1Multipart.POST("/upload", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	// /test/json with multipart → 415 (JSON-locked group rejects).
	req := httptest.NewRequest(http.MethodPost, "/test/json", strings.NewReader(""))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xyz")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("JSON-locked /test/json with multipart: status = %d, want 415; body = %s",
			rec.Code, rec.Body.String())
	}

	// /test/upload with multipart → 200 (multipart sub-group accepts).
	req = httptest.NewRequest(http.MethodPost, "/test/upload", strings.NewReader(""))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xyz")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("multipart /test/upload: status = %d, want 200; body = %s",
			rec.Code, rec.Body.String())
	}
}
