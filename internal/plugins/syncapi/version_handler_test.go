package syncapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestVersionHandler_ReturnsEnvValue pins C-VER1's primary contract: the
// handler returns whatever CHRONICLE_VERSION is set to. We read directly
// from the env var (no flag/build symbol) so this is the only behavior
// that can drift, and we want a regression test for it.
func TestVersionHandler_ReturnsEnvValue(t *testing.T) {
	t.Setenv("CHRONICLE_VERSION", "0.1.2-test")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := VersionHandler(c); err != nil {
		t.Fatalf("VersionHandler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v (raw: %q)", err, rec.Body.String())
	}
	if got := body["version"]; got != "0.1.2-test" {
		t.Errorf("version = %q, want %q", got, "0.1.2-test")
	}
}

// TestVersionHandler_FallbackToUnknown pins the "no env var set" path.
// The handler must not 500, must not return an empty string, must
// return the documented fallback "unknown" — same fallback as the
// pre-migration backup manifest writer.
func TestVersionHandler_FallbackToUnknown(t *testing.T) {
	t.Setenv("CHRONICLE_VERSION", "")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := VersionHandler(c); err != nil {
		t.Fatalf("VersionHandler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v (raw: %q)", err, rec.Body.String())
	}
	if got := body["version"]; got != "unknown" {
		t.Errorf("version = %q, want %q", got, "unknown")
	}
}
