// Package hostinfo reports the identity of the RUNNING Chronicle process:
// which binary it is, which source revision produced it (when that was
// recorded at all), when the process started, and how long it has been up.
//
// WHY this package exists. On 2026-08-11 an operator deployed and saw no
// change. Diagnosing it took an hour of shell archaeology and produced a wrong
// conclusion: the container image's labels said
// `org.opencontainers.image.revision=33f4cb07` and `created=2026-02-19`, so the
// running binary was declared six months stale. It was not — it had been built
// minutes earlier. The labels were not lying about the image they described;
// they simply were not about this process. An image label is a claim made by
// whoever last wrote a tag, and `docker inspect <tag>` answers for whichever
// image holds that tag NOW, not for the image a running container was created
// from. Chronicle could fingerprint every installed system package down to the
// sha256 of each served file, and could say nothing whatsoever about itself.
//
// The rule this package encodes: only the process can testify about itself.
// Every fact here is read from inside it — debug.ReadBuildInfo(),
// os.Executable() + os.Stat, os.Hostname, os.Getpid, and a package-init
// timestamp — and every fact carries whether it is actually known, so a caller
// can print "not stamped" instead of a blank or a plausible-looking guess.
//
// It is a leaf: standard library only. That is deliberate, so both the operator
// diagnostics (internal/systems) and the public version endpoint
// (internal/plugins/syncapi) can import it without a cycle and without a
// second, drifting copy of the same parsing.
package hostinfo

import (
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// EnvVersion is the environment variable an operator or release pipeline can
// set to name the build explicitly. It is the highest-precedence source
// because it is the only one a human deliberately chose.
const EnvVersion = "CHRONICLE_VERSION"

// VersionUnknown is what Chronicle reports when NOTHING identifies the build.
// It is deliberately a visible sentinel rather than an empty string: "unknown"
// tells a reader the question was asked and had no answer, whereas "" reads as
// a rendering bug.
const VersionUnknown = "unknown"

// processStart is captured at package init, which Go runs before main(). It is
// therefore within milliseconds of true process start — not identical to it,
// and the diagnostics say so rather than claiming exactness. There is no
// portable way to ask the kernel for a Go process's own start time, and a
// wrong-by-milliseconds uptime is worth far more than no uptime at all.
var processStart = time.Now()

// ProcessStart returns when this process's Go packages were initialised.
func ProcessStart() time.Time { return processStart }

// Uptime returns how long this process has been running.
func Uptime() time.Duration { return time.Since(processStart) }

// Build is the compile-time identity of the running binary. Every field that
// can be absent is paired with a "do we know?" flag, because the whole point of
// this package is to distinguish "clean tree" from "nobody recorded whether the
// tree was clean".
type Build struct {
	// InfoOK is false when debug.ReadBuildInfo() itself returned nothing —
	// i.e. the binary was not built by a module-aware Go toolchain. Everything
	// else in this struct is then zero and means nothing.
	InfoOK bool

	// Stamped is true when the toolchain embedded VCS settings (vcs.revision).
	// It is false in several very different situations that must never be
	// conflated with "the binary is old": the build environment had no VCS
	// tool on PATH (Go skips stamping SILENTLY in that case), the build ran
	// outside a checkout, or -buildvcs=false was passed. Chronicle's Docker
	// builder installs git and sets safe.directory, so images from the current
	// Dockerfile ARE stamped; an unstamped one predates that change or was
	// built from a context without .git. Absent is absent, not stale.
	Stamped bool

	Revision     string // full git SHA from vcs.revision, "" when not stamped
	RevisionTime string // vcs.time, the COMMITTER date in UTC RFC3339, "" when not stamped

	// Modified mirrors vcs.modified: the working tree was dirty at build time.
	// Note it counts UNTRACKED files too (gitignored ones do not count), so a
	// stray scratch directory is enough to flip it.
	Modified bool
	// ModifiedKnown separates "clean" from "unrecorded"; without it a false
	// Modified would advertise a clean tree we never observed.
	ModifiedKnown bool

	GoVersion   string // e.g. "go1.24.7"
	MainPath    string // main module path, e.g. "github.com/keyxmakerx/chronicle"
	MainVersion string // "(devel)" when unstamped; a v0.0.0-<time>-<rev> pseudo-version when stamped
	GOOS        string
	GOARCH      string
}

// ShortRevision is the first 12 hex characters of the revision — the form git
// itself prints and the form that fits in a version string. Empty stays empty.
func (b Build) ShortRevision() string {
	if len(b.Revision) > 12 {
		return b.Revision[:12]
	}
	return b.Revision
}

// readBuildOnce memoises the parse: build identity cannot change while the
// process runs, and the diagnostics may ask repeatedly.
var readBuildOnce = sync.OnceValue(func() Build { return buildFrom(debug.ReadBuildInfo()) })

// ReadBuild returns the running binary's compile-time identity.
func ReadBuild() Build { return readBuildOnce() }

// buildFrom is the pure parse, split out so both the stamped and the unstamped
// paths are testable — the unstamped path is the one that actually ships today
// (and the one a `go test` binary exercises), so it must be the tested one.
func buildFrom(bi *debug.BuildInfo, ok bool) Build {
	if !ok || bi == nil {
		return Build{}
	}
	b := Build{
		InfoOK:      true,
		GoVersion:   bi.GoVersion,
		MainPath:    bi.Main.Path,
		MainVersion: bi.Main.Version,
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			b.Revision = s.Value
		case "vcs.time":
			b.RevisionTime = s.Value
		case "vcs.modified":
			b.Modified = s.Value == "true"
			b.ModifiedKnown = true
		case "GOOS":
			b.GOOS = s.Value
		case "GOARCH":
			b.GOARCH = s.Value
		}
	}
	// A revision is what makes the stamp useful; vcs.time or vcs.modified
	// without it would not identify anything.
	b.Stamped = b.Revision != ""
	return b
}

// Executable is the on-disk identity of the file this process is executing.
// This is the fallback that ALWAYS works — it needs no build-time cooperation,
// survives the missing-git problem entirely, and its mtime is the closest
// honest proxy for "when was this built" (it is what actually settled the
// 2026-08-11 incident).
type Executable struct {
	Path    string
	Size    int64
	ModTime time.Time
	// Err is non-empty when the path could not be resolved or stat'd; the
	// other fields are then zero and must not be printed as facts.
	Err string
}

// ReadExecutable stats the running binary. Not memoised: it is one syscall
// pair, and a fresh stat is the honest answer if the file was replaced under a
// running process.
func ReadExecutable() Executable {
	p, err := os.Executable()
	if err != nil {
		return Executable{Err: "os.Executable: " + err.Error()}
	}
	fi, err := os.Stat(p)
	if err != nil {
		return Executable{Path: p, Err: "stat: " + err.Error()}
	}
	return Executable{Path: p, Size: fi.Size(), ModTime: fi.ModTime()}
}

// Version resolves the single version string Chronicle reports to external
// clients (GET /api/version, consumed by the Foundry VTT module's dashboard).
//
// Precedence — CHRONICLE_VERSION, then the compiled-in VCS revision, then the
// main module version, then the "unknown" sentinel. Before this existed the
// endpoint read the env var alone, and because no Dockerfile, Makefile or
// workflow set that variable, EVERY shipped image answered the literal string
// "unknown". CI now passes it for tag builds only, so the VCS revision is what
// answers for an ordinary branch build.
func Version() string { return VersionFrom(os.Getenv(EnvVersion), ReadBuild()) }

// VersionFrom is the pure precedence. It is exported for two reasons: it is
// what lets the STAMPED branch be pinned at all (a `go test` binary is never
// stamped, so the live endpoint test can only reach the fallback), and it lets
// the host.build diagnostic print what /api/version will answer by CALLING the
// rule rather than restating it — a restated copy is a copy that can disagree.
func VersionFrom(env string, b Build) string {
	if s := strings.TrimSpace(env); s != "" {
		return s
	}
	if b.Revision != "" {
		v := b.ShortRevision()
		if b.ModifiedKnown && b.Modified {
			v += "-dirty"
		}
		return v
	}
	// "(devel)" is the toolchain's own sentinel for "no version information",
	// so passing it through would just be "unknown" wearing a different hat.
	if b.MainVersion != "" && b.MainVersion != "(devel)" {
		return b.MainVersion
	}
	return VersionUnknown
}
