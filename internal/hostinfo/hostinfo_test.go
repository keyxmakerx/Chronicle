package hostinfo

import (
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// TestBuildFrom covers the parse, and covers the DEGRADED paths first because
// they still ship: Go skips VCS stamping SILENTLY whenever the build ran
// outside a checkout, without a VCS tool on PATH, or with -buildvcs=false, and
// every `go test` binary is unstamped for the first of those reasons. (The
// Docker builder was given git on 2026-08-11, so images from the current
// Dockerfile take the stamped path — measured, `go version -m` reports
// vcs.revision.) A test that only exercised the stamped path would pass forever
// while a real binary reported nothing.
func TestBuildFrom(t *testing.T) {
	tests := []struct {
		name string
		bi   *debug.BuildInfo
		ok   bool
		want Build
	}{
		{
			name: "ReadBuildInfo failed entirely",
			bi:   nil,
			ok:   false,
			want: Build{}, // InfoOK false — every other field means nothing
		},
		{
			name: "nil BuildInfo with ok=true is still treated as no info",
			bi:   nil,
			ok:   true,
			want: Build{},
		},
		{
			name: "unstamped: the shipped Docker case, and the go-test case",
			bi: &debug.BuildInfo{
				GoVersion: "go1.24.7",
				Main:      debug.Module{Path: "github.com/keyxmakerx/chronicle", Version: "(devel)"},
				Settings: []debug.BuildSetting{
					{Key: "-buildmode", Value: "exe"},
					{Key: "GOOS", Value: "linux"},
					{Key: "GOARCH", Value: "amd64"},
				},
			},
			ok: true,
			want: Build{
				InfoOK:      true,
				Stamped:     false,
				GoVersion:   "go1.24.7",
				MainPath:    "github.com/keyxmakerx/chronicle",
				MainVersion: "(devel)",
				GOOS:        "linux",
				GOARCH:      "amd64",
			},
		},
		{
			name: "stamped clean tree",
			bi: &debug.BuildInfo{
				GoVersion: "go1.24.7",
				Main:      debug.Module{Path: "github.com/keyxmakerx/chronicle", Version: "v0.0.0-20260810181157-84e31334d5b9"},
				Settings: []debug.BuildSetting{
					{Key: "vcs", Value: "git"},
					{Key: "vcs.revision", Value: "84e31334d5b9ff78b940fe452efc502cbae8f707"},
					{Key: "vcs.time", Value: "2026-08-10T18:11:57Z"},
					{Key: "vcs.modified", Value: "false"},
					{Key: "GOOS", Value: "linux"},
					{Key: "GOARCH", Value: "amd64"},
				},
			},
			ok: true,
			want: Build{
				InfoOK:        true,
				Stamped:       true,
				Revision:      "84e31334d5b9ff78b940fe452efc502cbae8f707",
				RevisionTime:  "2026-08-10T18:11:57Z",
				Modified:      false,
				ModifiedKnown: true,
				GoVersion:     "go1.24.7",
				MainPath:      "github.com/keyxmakerx/chronicle",
				MainVersion:   "v0.0.0-20260810181157-84e31334d5b9",
				GOOS:          "linux",
				GOARCH:        "amd64",
			},
		},
		{
			name: "stamped dirty tree",
			bi: &debug.BuildInfo{
				GoVersion: "go1.24.7",
				Main:      debug.Module{Path: "github.com/keyxmakerx/chronicle", Version: "v0.0.0-20260810181157-84e31334d5b9+dirty"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "84e31334d5b9ff78b940fe452efc502cbae8f707"},
					{Key: "vcs.modified", Value: "true"},
				},
			},
			ok: true,
			want: Build{
				InfoOK:        true,
				Stamped:       true,
				Revision:      "84e31334d5b9ff78b940fe452efc502cbae8f707",
				Modified:      true,
				ModifiedKnown: true,
				GoVersion:     "go1.24.7",
				MainPath:      "github.com/keyxmakerx/chronicle",
				MainVersion:   "v0.0.0-20260810181157-84e31334d5b9+dirty",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildFrom(tc.bi, tc.ok)
			if got != tc.want {
				t.Errorf("buildFrom() =\n  %+v\nwant\n  %+v", got, tc.want)
			}
		})
	}
}

// TestBuildFrom_ModifiedKnownSeparatesCleanFromUnrecorded is the assertion that
// keeps the diagnostics honest: without ModifiedKnown, an unstamped build would
// present Modified=false and the UI would advertise "clean tree at build" for a
// tree nobody ever looked at.
func TestBuildFrom_ModifiedKnownSeparatesCleanFromUnrecorded(t *testing.T) {
	unstamped := buildFrom(&debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true)
	if unstamped.Modified {
		t.Errorf("unstamped build reported Modified=true")
	}
	if unstamped.ModifiedKnown {
		t.Errorf("unstamped build claimed to KNOW the tree was clean; ModifiedKnown must be false")
	}

	clean := buildFrom(&debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "abc"},
		{Key: "vcs.modified", Value: "false"},
	}}, true)
	if !clean.ModifiedKnown || clean.Modified {
		t.Errorf("stamped clean build: got Modified=%v ModifiedKnown=%v, want false/true", clean.Modified, clean.ModifiedKnown)
	}
}

func TestShortRevision(t *testing.T) {
	tests := []struct {
		name string
		rev  string
		want string
	}{
		{"empty stays empty", "", ""},
		{"full sha truncates to 12", "84e31334d5b9ff78b940fe452efc502cbae8f707", "84e31334d5b9"},
		{"already short is untouched", "84e3133", "84e3133"},
		{"exactly 12 is untouched", "84e31334d5b9", "84e31334d5b9"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Build{Revision: tc.rev}).ShortRevision(); got != tc.want {
				t.Errorf("ShortRevision(%q) = %q, want %q", tc.rev, got, tc.want)
			}
		})
	}
}

// TestVersionFrom pins the precedence chain that GET /api/version now follows.
// This is the only place the STAMPED branch can be pinned at all: a `go test`
// binary carries no vcs.* settings (measured — debug.ReadBuildInfo in a test
// binary reports Main.Version "(devel)" and zero vcs keys), so the live
// endpoint test can only ever reach the fallback.
func TestVersionFrom(t *testing.T) {
	stamped := Build{InfoOK: true, Stamped: true, Revision: "84e31334d5b9ff78b940fe452efc502cbae8f707", MainVersion: "v0.0.0-20260810181157-84e31334d5b9"}
	dirty := stamped
	dirty.Modified, dirty.ModifiedKnown = true, true

	tests := []struct {
		name  string
		env   string
		build Build
		want  string
	}{
		{"env wins over everything", "1.4.0", stamped, "1.4.0"},
		{"env wins even with no build info", "1.4.0", Build{}, "1.4.0"},
		{"whitespace-only env is not a version", "   ", stamped, "84e31334d5b9"},
		{"env is trimmed", "  1.4.0\n", Build{}, "1.4.0"},
		{"unset env falls back to short revision", "", stamped, "84e31334d5b9"},
		{"dirty build is marked dirty", "", dirty, "84e31334d5b9-dirty"},
		{
			name:  "no revision falls back to a real module version",
			env:   "",
			build: Build{InfoOK: true, MainVersion: "v1.2.3"},
			want:  "v1.2.3",
		},
		{
			name:  "(devel) is the toolchain's own unknown, not a version",
			env:   "",
			build: Build{InfoOK: true, MainVersion: "(devel)"},
			want:  VersionUnknown,
		},
		{
			name:  "nothing known at all — the case every shipped image hits today",
			env:   "",
			build: Build{},
			want:  VersionUnknown,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := VersionFrom(tc.env, tc.build); got != tc.want {
				t.Errorf("VersionFrom(%q, %+v) = %q, want %q", tc.env, tc.build, got, tc.want)
			}
		})
	}
}

// TestVersionNeverEmpty guards the property the Foundry module depends on: the
// endpoint must always have SOMETHING to display. An empty string would render
// as "Connected to Chronicle v" and read as a UI bug rather than a gap.
func TestVersionNeverEmpty(t *testing.T) {
	t.Setenv(EnvVersion, "")
	if got := Version(); got == "" {
		t.Fatalf("Version() returned an empty string; it must return the %q sentinel instead", VersionUnknown)
	}
}

// TestReadExecutable asserts the always-available fallback actually works in
// this environment — under `go test` the executable is the test binary, which
// is exactly the shape of the question ("what file am I running?").
func TestReadExecutable(t *testing.T) {
	exe := ReadExecutable()
	if exe.Err != "" {
		// Not a hard failure: some sandboxes genuinely cannot resolve it. But
		// the struct must then carry the reason instead of silent zeroes.
		t.Logf("ReadExecutable degraded: %s", exe.Err)
		return
	}
	if exe.Path == "" {
		t.Error("no error but empty Path")
	}
	if exe.Size <= 0 {
		t.Errorf("no error but Size = %d", exe.Size)
	}
	if exe.ModTime.IsZero() {
		t.Error("no error but zero ModTime")
	}
}

// TestProcessStartAndUptime pins that uptime is measured from a fixed origin
// rather than recomputed as time.Now() each call (which would always be ~0).
func TestProcessStartAndUptime(t *testing.T) {
	if ProcessStart().IsZero() {
		t.Fatal("ProcessStart is zero; package init did not run?")
	}
	if ProcessStart().After(time.Now()) {
		t.Error("ProcessStart is in the future")
	}
	first := Uptime()
	time.Sleep(2 * time.Millisecond)
	if second := Uptime(); second <= first {
		t.Errorf("Uptime did not advance: %v then %v", first, second)
	}
}

// TestReadBuildIsMemoised documents that repeated diagnostic runs do not re-parse.
func TestReadBuildIsMemoised(t *testing.T) {
	a, b := ReadBuild(), ReadBuild()
	if a != b {
		t.Errorf("ReadBuild() not stable across calls: %+v vs %+v", a, b)
	}
	// Also record what THIS binary actually reports, so a failing CI run shows
	// whether the test environment is stamped or not.
	t.Logf("this test binary: InfoOK=%v Stamped=%v MainVersion=%q GoVersion=%q",
		a.InfoOK, a.Stamped, a.MainVersion, a.GoVersion)
	if a.Stamped && !strings.HasPrefix(a.MainVersion, "v") {
		t.Logf("stamped build with non-pseudo main version %q", a.MainVersion)
	}
}
