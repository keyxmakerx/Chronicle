package syncapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

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

// TestVersionHandler_ReturnsEnvValue pins that CHRONICLE_VERSION always wins:
// it is the only source a human deliberately chose, so nothing derived from
// the binary may override it.
func TestVersionHandler_ReturnsEnvValue(t *testing.T) {
	t.Setenv("CHRONICLE_VERSION", "0.1.2-test")

	if got := getVersion(t); got != "0.1.2-test" {
		t.Errorf("version = %q, want %q", got, "0.1.2-test")
	}
}

// TestVersionHandler_FallsBackBeyondTheEnvVar asserts the fallback chain
// (VCS revision, then "unknown") rather than a fixed literal, since which
// branch it lands on depends on how the test binary was built. A `go test`
// binary carries no vcs.* build info, so this exercises the unstamped
// branch; the stamped branch is pinned by the precedence table in
// internal/hostinfo.
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
