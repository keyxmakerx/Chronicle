package systems

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/hostinfo"
)

// fixed reference points so every assertion below is deterministic.
var (
	testNow   = time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	testStart = testNow.Add(-3*time.Hour - 12*time.Minute)
)

func stampedBuild() hostinfo.Build {
	return hostinfo.Build{
		InfoOK:        true,
		Stamped:       true,
		Revision:      "84e31334d5b9ff78b940fe452efc502cbae8f707",
		RevisionTime:  "2026-08-10T18:11:57Z",
		ModifiedKnown: true,
		GoVersion:     "go1.24.7",
		MainPath:      "github.com/keyxmakerx/chronicle",
		MainVersion:   "v0.0.0-20260810181157-84e31334d5b9",
		GOOS:          "linux",
		GOARCH:        "amd64",
	}
}

// unstampedBuild is what EVERY image built by the current Docker builder
// reports: golang:1.24-alpine has no git, so Go skips VCS stamping silently.
func unstampedBuild() hostinfo.Build {
	return hostinfo.Build{
		InfoOK:      true,
		GoVersion:   "go1.24.7",
		MainPath:    "github.com/keyxmakerx/chronicle",
		MainVersion: "(devel)",
		GOOS:        "linux",
		GOARCH:      "amd64",
	}
}

func okExecutable() hostinfo.Executable {
	return hostinfo.Executable{
		Path:    "/chronicle",
		Size:    63_000_000,
		ModTime: testNow.Add(-4 * time.Hour),
	}
}

func okProcess() hostProcess {
	return hostProcess{Hostname: "3f2a19b0c4d1", PID: 1, Start: testStart}
}

// TestHostDiagnosticsRegistered pins that the catalog actually exposes them —
// a renderer nobody can reach is not a diagnostic.
func TestHostDiagnosticsRegistered(t *testing.T) {
	cat := diagnosticCatalog()

	tests := []struct {
		name  string
		title string
	}{
		{name: "host.build"},
		{name: "host.runtime"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var found *Diagnostic
			for i := range cat {
				if cat[i].Name == tc.name {
					found = &cat[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("%s is not registered in diagnosticCatalog()", tc.name)
			}
			if found.Run == nil {
				t.Errorf("%s has a nil Run", tc.name)
			}
			if found.Title == "" || found.Desc == "" {
				t.Errorf("%s: Title=%q Desc=%q — the catalog listing is how the assistant chooses", tc.name, found.Title, found.Desc)
			}
			if found.ArgHint != "" {
				t.Errorf("%s takes no argument but advertises ArgHint %q", tc.name, found.ArgHint)
			}
			if found.FullDump {
				t.Errorf("%s is cheap and read-only; gating it behind full_dump would hide the first thing an operator should run", tc.name)
			}
		})
	}
}

// TestHostBuildPrecedesServedContentDiagnostics pins the ORDER, which is a real
// decision and not cosmetics: every system.*/packages.* diagnostic describes
// what the server is serving, and all of them are worthless if the server is
// not the build you think it is. host.build has to be reachable before them.
func TestHostBuildPrecedesServedContentDiagnostics(t *testing.T) {
	cat := diagnosticCatalog()
	hostIdx := -1
	for i, d := range cat {
		if d.Name == "host.build" {
			hostIdx = i
			break
		}
	}
	if hostIdx == -1 {
		t.Fatal("host.build is not in the catalog")
	}
	for i, d := range cat {
		if (strings.HasPrefix(d.Name, "system.") || strings.HasPrefix(d.Name, "packages.")) && i < hostIdx {
			t.Errorf("%s (index %d) is listed before host.build (index %d)", d.Name, i, hostIdx)
		}
	}
}

// TestHostDiagnosticsRunLive exercises the real, unmocked path — including
// runtime.ReadMemStats and os.Executable — and asserts it neither panics nor
// returns nothing. It runs each twice because host.build memoises its build
// info and a second call must still render.
func TestHostDiagnosticsRunLive(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		heading string
	}{
		{name: "host.build", heading: "## host.build"},
		{name: "host.runtime", heading: "## host.runtime"},
		// The catalog passes the raw ?arg= through; neither diagnostic takes
		// one, so a stray argument must be harmless rather than fatal.
		{name: "host.build", arg: "unexpected-arg", heading: "## host.build"},
	}

	for _, tc := range tests {
		t.Run(tc.name+"/"+tc.arg, func(t *testing.T) {
			for i := 0; i < 2; i++ {
				got, ok := RunDiagnostic(diagnosticCatalog(), tc.name, tc.arg)
				if !ok {
					t.Fatalf("RunDiagnostic(%q) reported no such diagnostic", tc.name)
				}
				if !strings.HasPrefix(got, tc.heading) {
					t.Errorf("call %d: output does not start with %q; got:\n%s", i, tc.heading, first(got, 200))
				}
				if len(got) < 200 {
					t.Errorf("call %d: suspiciously short output (%d bytes):\n%s", i, len(got), got)
				}
			}
		})
	}
}

// TestRenderHostBuildDegradedPaths is the core of this file. The DEGRADED paths
// are the ones that ship — an unstamped binary is what the Docker builder
// produces today — so the assertion that matters is that absence prints the
// honest sentence rather than a blank field or a confident-looking zero.
func TestRenderHostBuildDegradedPaths(t *testing.T) {
	tests := []struct {
		name       string
		build      hostinfo.Build
		exe        hostinfo.Executable
		env        string
		proc       hostProcess
		contains   []string
		notContain []string
	}{
		{
			name:  "unstamped binary — the shipped Docker case",
			build: unstampedBuild(),
			exe:   okExecutable(),
			proc:  okProcess(),
			contains: []string{
				notStampedHeadline,
				"Absent is not stale",
				"no `git`",
				"apk add --no-cache git",
				// the always-available fallback must still be there
				"path: `/chronicle`",
				"mtime:",
			},
			notContain: []string{
				"working tree at build: clean",
				"revision: ``",
				"commit time: \n",
			},
		},
		{
			name:  "debug.ReadBuildInfo returned nothing at all",
			build: hostinfo.Build{},
			exe:   okExecutable(),
			proc:  okProcess(),
			contains: []string{
				notStampedHeadline,
				"debug.ReadBuildInfo()",
				"### Go toolchain and module",
				"unavailable",
			},
			notContain: []string{
				"go version: \n",
				"main module: ``",
			},
		},
		{
			name:  "executable could not be inspected",
			build: unstampedBuild(),
			exe:   hostinfo.Executable{Path: "/chronicle", Err: "stat: permission denied"},
			proc:  okProcess(),
			contains: []string{
				"could not be inspected",
				"stat: permission denied",
				"unknown, not zero",
			},
			notContain: []string{
				"size: 0 bytes",
				"mtime: 0001-01-01",
			},
		},
		{
			name:  "hostname lookup failed",
			build: unstampedBuild(),
			exe:   okExecutable(),
			proc:  hostProcess{PID: 1, Start: testStart, HostnameErr: "lookup failed"},
			contains: []string{
				"hostname: _could not be determined_",
				"lookup failed",
			},
			notContain: []string{"hostname: ``"},
		},
		{
			name:  "stamped but vcs.modified absent — must not claim the tree was clean",
			build: func() hostinfo.Build { b := stampedBuild(); b.ModifiedKnown = false; return b }(),
			exe:   okExecutable(),
			proc:  okProcess(),
			contains: []string{
				"working tree at build: _not recorded_",
				"do not read this as",
			},
			notContain: []string{
				"working tree at build: clean",
				"DIRTY",
			},
		},
		{
			name:  "stamped clean tree",
			build: stampedBuild(),
			exe:   okExecutable(),
			proc:  okProcess(),
			contains: []string{
				"revision: `84e31334d5b9ff78b940fe452efc502cbae8f707`",
				"commit time: 2026-08-10T18:11:57Z",
				"working tree at build: clean",
			},
			// Only the headline: the trust note legitimately quotes the phrase
			// "not stamped" while explaining how to read it.
			notContain: []string{notStampedHeadline},
		},
		{
			name:  "stamped dirty tree",
			build: func() hostinfo.Build { b := stampedBuild(); b.Modified = true; return b }(),
			exe:   okExecutable(),
			proc:  okProcess(),
			contains: []string{
				"working tree at build: **DIRTY**",
				"uncommitted changes",
			},
			notContain: []string{"working tree at build: clean"},
		},
		{
			name:  "CHRONICLE_VERSION unset — reports the honest unknown",
			build: unstampedBuild(),
			exe:   okExecutable(),
			env:   "",
			proc:  okProcess(),
			contains: []string{
				"`CHRONICLE_VERSION`: **(unset)**",
				"`GET /api/version` therefore reports: **unknown**",
			},
		},
		{
			name:  "CHRONICLE_VERSION set — wins over the stamps",
			build: stampedBuild(),
			exe:   okExecutable(),
			env:   "1.4.0",
			proc:  okProcess(),
			contains: []string{
				"`CHRONICLE_VERSION`: `1.4.0`",
				"reports: **1.4.0**",
			},
			notContain: []string{"reports: **unknown**"},
		},
		{
			name:  "unset env with a stamped binary falls back to the revision",
			build: stampedBuild(),
			exe:   okExecutable(),
			env:   "",
			proc:  okProcess(),
			contains: []string{
				"reports: **84e31334d5b9**",
			},
			notContain: []string{"reports: **unknown**"},
		},
		{
			name:  "executable mtime in the future is called out as clock skew",
			build: unstampedBuild(),
			exe:   hostinfo.Executable{Path: "/chronicle", Size: 10, ModTime: testNow.Add(2 * time.Hour)},
			proc:  okProcess(),
			contains: []string{"in the FUTURE"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := renderHostBuildFrom(tc.build, tc.exe, tc.env, tc.proc, testNow)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output is missing %q\n--- output ---\n%s", want, got)
				}
			}
			for _, bad := range tc.notContain {
				if strings.Contains(got, bad) {
					t.Errorf("output must not contain %q\n--- output ---\n%s", bad, got)
				}
			}
		})
	}
}

// TestRenderHostBuildNeverPrintsAnEmptyValue is the generalized form of the
// rule: for the most degraded input possible, no line may end in a label with
// nothing after it and no code span may be empty. This is what stops a future
// edit from replacing "not stamped" with a blank and passing the other tests.
func TestRenderHostBuildNeverPrintsAnEmptyValue(t *testing.T) {
	got := renderHostBuildFrom(hostinfo.Build{}, hostinfo.Executable{Err: "os.Executable: not supported"}, "", hostProcess{HostnameErr: "no hostname"}, testNow)

	inFence := false
	for _, line := range strings.Split(got, "\n") {
		// ``` fences contain "``" as a substring, so they are skipped rather
		// than reported as empty code spans.
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.Contains(line, "``") {
			t.Errorf("line contains an empty code span: %q", line)
		}
		// A bullet of at most four words ending in a colon is a field label
		// whose value went missing ("- go version:"). Longer sentences ending
		// in a colon are prose introducing the code block below and are fine.
		trimmed := strings.TrimRight(line, " ")
		if strings.HasPrefix(trimmed, "- ") && strings.HasSuffix(trimmed, ":") && len(strings.Fields(trimmed)) <= 5 {
			t.Errorf("line ends with a label and no value: %q", line)
		}
	}
	// A zero process start must not be rendered as a year-1 date or a
	// 2562047-hour uptime — both look like data and are not.
	for _, bad := range []string{"0001-01-01", "2562047h"} {
		if strings.Contains(got, bad) {
			t.Errorf("degraded output contains nonsense value %q\n%s", bad, got)
		}
	}
	// And the honest sentence must be the thing that filled the gap.
	if !strings.Contains(got, notStampedHeadline) {
		t.Errorf("fully-degraded output does not carry the not-stamped wording\n%s", got)
	}
}

// TestRenderHostBuildNamesTheDockerLabelTrap pins the note that exists solely
// so the next reader does not repeat the 2026-08-11 misdiagnosis: image labels
// were read as evidence about the running binary. Deleting this note would
// silently remove the only thing standing between a reader and that hour.
func TestRenderHostBuildNamesTheDockerLabelTrap(t *testing.T) {
	got := renderHostBuildFrom(unstampedBuild(), okExecutable(), "", okProcess(), testNow)
	for _, want := range []string{
		"org.opencontainers.image.revision",
		"33f4cb07",
		"docker inspect --format '{{.Image}}'",
		"NOT evidence about this process",
		"//go:embed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the trust note lost %q\n--- output ---\n%s", want, got)
		}
	}
}

// TestRenderHostBuildSurvivesRedaction guards the render-layer interaction:
// every diagnostic result is passed through redactSecrets, and a heuristic that
// mangled the revision or the not-stamped sentence would defeat the diagnostic
// while leaving every other test green.
func TestRenderHostBuildSurvivesRedaction(t *testing.T) {
	for _, b := range []hostinfo.Build{stampedBuild(), unstampedBuild(), {}} {
		got := redactSecrets(renderHostBuildFrom(b, okExecutable(), "", okProcess(), testNow))
		if strings.Contains(got, "[REDACTED]") {
			t.Errorf("redactSecrets fired on host.build output; it should have nothing to redact:\n%s", got)
		}
		if b.Stamped && !strings.Contains(got, b.Revision) {
			t.Errorf("revision was mangled by redaction:\n%s", got)
		}
	}
}

// TestRenderHostRuntime covers the runtime snapshot including the never-GC'd
// path, which a freshly started process really does hit.
func TestRenderHostRuntime(t *testing.T) {
	tests := []struct {
		name       string
		ms         runtime.MemStats
		counts     runtimeCounts
		contains   []string
		notContain []string
	}{
		{
			name:   "fresh process, no GC yet",
			ms:     runtime.MemStats{Alloc: 2 << 20, HeapInuse: 3 << 20, Sys: 12 << 20},
			counts: runtimeCounts{Goroutines: 7, NumCPU: 4, GOMAXPROCS: 4},
			contains: []string{
				"uptime: **3h12m0s**",
				"goroutines: **7**",
				"NumCPU: 4  ·  GOMAXPROCS: 4",
				"completed cycles: 0",
				"no GC has run yet",
			},
			notContain: []string{
				"last GC:",
				"GOMAXPROCS is below NumCPU",
			},
		},
		{
			name: "after several GCs",
			ms: func() runtime.MemStats {
				ms := runtime.MemStats{Alloc: 40 << 20, HeapInuse: 50 << 20, Sys: 200 << 20, HeapObjects: 1234, TotalAlloc: 900 << 20, NumGC: 3, PauseTotalNs: 3_000_000, GCCPUFraction: 0.0012}
				ms.LastGC = uint64(testNow.Add(-30 * time.Second).UnixNano())
				ms.PauseNs[0] = 500_000   // oldest of the three
				ms.PauseNs[1] = 1_200_000 // ...
				ms.PauseNs[2] = 1_300_000 // newest (index NumGC-1)
				return ms
			}(),
			counts: runtimeCounts{Goroutines: 91, NumCPU: 8, GOMAXPROCS: 2},
			contains: []string{
				"completed cycles: 3",
				"30s ago",
				"total STW pause: 3ms",
				"mean per cycle: 1ms",
				"GC CPU fraction: 0.1200%",
				"recent pauses (newest first): 1.3ms, 1.2ms, 500µs",
				"GOMAXPROCS is below NumCPU",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := renderHostRuntimeFrom(&tc.ms, tc.counts, testStart, testNow)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output is missing %q\n--- output ---\n%s", want, got)
				}
			}
			for _, bad := range tc.notContain {
				if strings.Contains(got, bad) {
					t.Errorf("output must not contain %q\n--- output ---\n%s", bad, got)
				}
			}
		})
	}
}

// TestRecentPauses covers the wrap in the runtime's 256-slot circular pause
// buffer — the index arithmetic is the only place here that can be silently
// wrong and still produce plausible numbers.
func TestRecentPauses(t *testing.T) {
	ring := func(numGC uint32, set map[int]uint64) *runtime.MemStats {
		ms := &runtime.MemStats{NumGC: numGC}
		for i, v := range set {
			ms.PauseNs[i] = v
		}
		return ms
	}

	tests := []struct {
		name string
		ms   *runtime.MemStats
		n    int
		want string
	}{
		{
			name: "no GC yet",
			ms:   ring(0, nil),
			n:    5,
			want: "_none_",
		},
		{
			name: "single cycle uses slot 0",
			ms:   ring(1, map[int]uint64{0: 700_000}),
			n:    5,
			want: "700µs",
		},
		{
			name: "asks for more than have happened",
			ms:   ring(2, map[int]uint64{0: 1_000_000, 1: 2_000_000}),
			n:    5,
			want: "2ms, 1ms",
		},
		{
			// 300 cycles: the newest lands at (300+255)%256 = 43, matching the
			// runtime's own documented index for the most recent pause.
			name: "wrapped ring, newest first",
			ms:   ring(300, map[int]uint64{43: 9_000_000, 42: 8_000_000, 41: 7_000_000}),
			n:    3,
			want: "9ms, 8ms, 7ms",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := recentPauses(tc.ms, tc.n); got != tc.want {
				t.Errorf("recentPauses = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRelativeToAndDurations(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"zero time is unknown, not 'since 1970'", relativeTo(time.Time{}, testNow), "unknown"},
		{"past", relativeTo(testNow.Add(-90*time.Second), testNow), "1m30s ago"},
		{"future is flagged, not negative", relativeTo(testNow.Add(time.Hour), testNow), "1h0m0s in the FUTURE — clock skew?"},
		{"sub-second duration", shortDuration(400 * time.Millisecond), "under 1s"},
		{"nanos below a millisecond render as µs", shortNanos(750_000), "750µs"},
		{"nanos above a millisecond render as ms", shortNanos(2_500_000), "2.5ms"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

// first trims long output in failure messages.
func first(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
