package syncapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/hostinfo"
)

// getVersion runs the handler and returns the decoded {"version": …} value.
// The JSON SHAPE is itself a contract — the Foundry VTT module reads this key —
// so every test goes through a real decode rather than a substring match.
func getVersion(t *testing.T) string {
	t.Helper()

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
	if _, ok := body["version"]; !ok {
		t.Fatalf("response has no \"version\" key: %q", rec.Body.String())
	}
	return body["version"]
}

// TestVersionHandler_ReturnsEnvValue pins C-VER1's primary contract, unchanged:
// CHRONICLE_VERSION still wins outright. It is the only source a human
// deliberately chose, so nothing derived from the binary may override it.
func TestVersionHandler_ReturnsEnvValue(t *testing.T) {
	t.Setenv("CHRONICLE_VERSION", "0.1.2-test")

	if got := getVersion(t); got != "0.1.2-test" {
		t.Errorf("version = %q, want %q", got, "0.1.2-test")
	}
}

// TestVersionHandler_FallsBackBeyondTheEnvVar replaces the old
// "unset ⇒ literally unknown" assertion. That assertion was true and was also
// the bug: CHRONICLE_VERSION is set by no Dockerfile, compose file, Makefile or
// workflow, so EVERY shipped image answered "unknown" and the Foundry dashboard
// displayed it. The handler now falls through to the VCS revision compiled into
// the binary, and only then to "unknown".
//
// This test asserts the fallback CHAIN rather than a fixed literal, because
// which branch it lands on depends on how the binary was built — and it asserts
// the right thing in either world instead of silently passing in one of them.
// Measured 2026-08-11: a `go test` binary carries no vcs.* build settings
// (debug.ReadBuildInfo reports Main.Version "(devel)" and zero vcs keys), so in
// practice this exercises the unstamped branch. The stamped branch cannot be
// produced from a test binary at all; it is pinned by the pure precedence table
// in internal/hostinfo.
func TestVersionHandler_FallsBackBeyondTheEnvVar(t *testing.T) {
	t.Setenv("CHRONICLE_VERSION", "")

	got := getVersion(t)
	if got == "" {
		t.Fatal("version is empty; the endpoint must always return a displayable string")
	}

	build := hostinfo.ReadBuild()
	t.Logf("this test binary: Stamped=%v MainVersion=%q → version %q", build.Stamped, build.MainVersion, got)

	if build.Revision == "" {
		// Nothing identifies the build: the honest sentinel must still appear.
		if got != "unknown" {
			t.Errorf("unstamped binary with no CHRONICLE_VERSION: version = %q, want %q", got, "unknown")
		}
		return
	}

	// The binary IS stamped, so "unknown" would now be a lie by omission.
	if got == "unknown" {
		t.Fatalf("binary carries revision %s but the endpoint still reported \"unknown\"", build.Revision)
	}
	if !strings.HasPrefix(build.Revision, strings.TrimSuffix(got, "-dirty")) {
		t.Errorf("version %q is not a prefix of the build revision %q", got, build.Revision)
	}
}

// TestVersionHandler_MatchesHostinfo pins that the endpoint and the host.build
// operator diagnostic can never disagree: both read the same resolver. A second
// copy of the precedence rule is a copy that drifts, and the version reported
// to an operator debugging a deploy is exactly the wrong place for a drift.
func TestVersionHandler_MatchesHostinfo(t *testing.T) {
	for _, env := range []string{"", "9.9.9"} {
		t.Setenv("CHRONICLE_VERSION", env)
		if got, want := getVersion(t), hostinfo.Version(); got != want {
			t.Errorf("CHRONICLE_VERSION=%q: endpoint says %q, hostinfo.Version() says %q", env, got, want)
		}
	}
}
