// operator_diag_host.go is the "which Chronicle am I actually running?" half of
// the operator diagnostic catalog.
//
// WHY it exists: Chronicle could already fingerprint every installed system
// PACKAGE down to the sha256 of each served file, and was completely blind to
// ITSELF. On 2026-08-11 that cost an hour of shell archaeology and produced a
// conclusion that had to be retracted — the container image's labels
// (org.opencontainers.image.revision=33f4cb07, created=2026-02-19) were read as
// evidence that the running binary was six months stale. It had been built
// minutes earlier. The labels described a different image that still held the
// tag in the deploy host's local image store; `docker inspect <tag>` never
// answers for the image a running container was created from.
//
// So these diagnostics report ONLY what the process can observe about itself,
// and each line says whether the fact is actually known. "not stamped" is a
// real answer; a blank field or a plausible-looking guess is not. The closing
// trust note names the label trap explicitly, because the next reader will have
// a `docker inspect` window open too.
package systems

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/hostinfo"
)

// hostBuildDiagnostic answers the first question of any "I deployed and nothing
// changed" investigation. Registered at the TOP of the catalog for that reason:
// every other diagnostic describes what the server is serving, and is worthless
// if the server itself is not the build you think it is.
func hostBuildDiagnostic() Diagnostic {
	return Diagnostic{
		Name:  "host.build",
		Title: "Which Chronicle binary is this process? (commit, build stamps, executable, uptime)",
		Desc:  "THE 'did my deploy actually land?' check: source revision, CHRONICLE_VERSION, the running executable's path/size/mtime, Go toolchain, process start + uptime, hostname, PID. Read from inside the process, so it answers where Docker image labels cannot — the output says which fields to trust and why.",
		Run:   func(string) string { return renderHostBuild() },
	}
}

// hostRuntimeDiagnostic is the cheap "is this process healthy right now?"
// snapshot — no FullDump gate, since it allocates nothing beyond the MemStats
// read and prints a fixed handful of lines.
func hostRuntimeDiagnostic() Diagnostic {
	return Diagnostic{
		Name:  "host.runtime",
		Title: "Process runtime snapshot (uptime, goroutines, memory, GC)",
		Desc:  "Uptime, goroutine count, NumCPU/GOMAXPROCS, a compact memory slice (Alloc / HeapInuse / Sys) and GC activity. Read-only and cheap — use for 'is it leaking / is it wedged / is it thrashing GC?'.",
		Run:   func(string) string { return renderHostRuntime() },
	}
}

// hostProcess is the process-identity slice of host.build. It lives here rather
// than in internal/hostinfo because a hostname lookup can fail and the failure
// has to be RENDERED ("could not be determined"), not swallowed into an empty
// string that reads like a missing field.
type hostProcess struct {
	Hostname    string
	HostnameErr string
	PID         int
	Start       time.Time
}

// readHostProcess gathers the live process identity.
func readHostProcess() hostProcess {
	p := hostProcess{PID: os.Getpid(), Start: hostinfo.ProcessStart()}
	h, err := os.Hostname()
	if err != nil {
		p.HostnameErr = err.Error()
	} else {
		p.Hostname = h
	}
	return p
}

// renderHostBuild wires the live sources into the pure renderer.
func renderHostBuild() string {
	return renderHostBuildFrom(
		hostinfo.ReadBuild(),
		hostinfo.ReadExecutable(),
		os.Getenv(hostinfo.EnvVersion),
		readHostProcess(),
		time.Now(),
	)
}

// renderHostBuildFrom is the pure renderer. Every input is injected so the
// DEGRADED paths — no VCS stamps, no executable, no hostname — can be tested
// directly. That matters more than usual here: the unstamped path is the one
// every shipped image takes, and a `go test` binary is unstamped too, so a
// renderer that only worked when stamps existed would look fine in CI forever.
func renderHostBuildFrom(b hostinfo.Build, exe hostinfo.Executable, envVersion string, proc hostProcess, now time.Time) string {
	var out strings.Builder
	out.WriteString("## host.build — which Chronicle binary is this process?\n\n")

	renderBuildRevision(&out, b)
	renderBuildEnvVersion(&out, envVersion, b)
	renderBuildExecutable(&out, exe, now)
	renderBuildToolchain(&out, b)
	renderBuildProcess(&out, proc, now)
	renderBuildTrustNote(&out)

	return out.String()
}

// notStampedHeadline is the exact wording for "the toolchain recorded no VCS
// metadata". It is a named constant because it is a CONTRACT: the whole point
// of this diagnostic is that an unknown revision prints this sentence instead
// of a blank, and the test asserts on it.
const notStampedHeadline = "**not stamped — the binary carries no VCS metadata.**"

// renderBuildRevision prints the source revision, or explains its absence.
// Absence is the common case today and it is NOT evidence of an old binary —
// saying so is the section's main job.
func renderBuildRevision(out *strings.Builder, b hostinfo.Build) {
	out.WriteString("### Source revision — from the VCS stamps compiled into this binary\n\n")
	switch {
	case !b.InfoOK:
		fmt.Fprintf(out, "- %s\n", notStampedHeadline)
		out.WriteString("- `debug.ReadBuildInfo()` returned nothing at all, so this binary was not produced by a module-aware Go build. Nothing here can identify the source.\n\n")
		return
	case !b.Stamped:
		fmt.Fprintf(out, "- %s\n", notStampedHeadline)
		out.WriteString("- Go records `vcs.revision` / `vcs.time` / `vcs.modified` only when the build ran inside a checkout **with the VCS tool on PATH**, and when it cannot it skips stamping **silently**: no warning, exit 0.\n")
		out.WriteString("- **Absent is not stale.** This says the fact was never recorded, not that the code is old. Use the executable mtime below for build recency instead.\n")
		out.WriteString("- Chronicle's Docker builder stage installs `git` and configures `safe.directory` for the build context, so an image produced by that Dockerfile **does** stamp. An unstamped binary is therefore one of: an image built before the builder had `git`; a build whose context carried no `.git` (a source tarball, or a `COPY` that excluded it); a build run with `-buildvcs=false`; or a `go test` / `go run` binary.\n")
		out.WriteString("- `CHRONICLE_VERSION` (below) names a release independently of the stamps, and is the one identifier a build can carry without a checkout.\n\n")
		return
	}
	fmt.Fprintf(out, "- revision: `%s`\n", b.Revision)
	if b.RevisionTime != "" {
		fmt.Fprintf(out, "- commit time: %s _(git committer date, UTC)_\n", b.RevisionTime)
	} else {
		out.WriteString("- commit time: _not recorded_\n")
	}
	switch {
	case !b.ModifiedKnown:
		out.WriteString("- working tree at build: _not recorded_ (the `vcs.modified` setting is absent — do not read this as \"clean\")\n")
	case b.Modified:
		out.WriteString("- working tree at build: **DIRTY** — the binary contains uncommitted changes, so the revision above does not fully describe it. (`vcs.modified` also counts *untracked* files; gitignored ones do not count.)\n")
	default:
		out.WriteString("- working tree at build: clean\n")
	}
	out.WriteString("- These stamps are compiled in by the toolchain. They cannot drift from the binary and are not metadata *about* it — when present, they are the strongest evidence available.\n\n")
}

// renderBuildEnvVersion prints CHRONICLE_VERSION and, crucially, what the
// public /api/version endpoint will therefore answer — the env var is that
// endpoint's highest-precedence input, and for years it was set by nothing at
// all, which is why the endpoint answered "unknown" for its whole life. CI now
// passes it for tag builds only, so "unset" remains the ordinary reading on a
// branch build and must not be rendered as a fault.
func renderBuildEnvVersion(out *strings.Builder, envVersion string, b hostinfo.Build) {
	out.WriteString("### CHRONICLE_VERSION\n\n")
	if strings.TrimSpace(envVersion) == "" {
		out.WriteString("- `CHRONICLE_VERSION`: **(unset)** — and unset is the NORMAL case, not a misconfiguration. The Dockerfile declares it as a build arg defaulting to empty, and CI passes a value only for `v*` tag builds; a branch push deliberately leaves it empty so that the moving tag `latest` is never reported as though it were a version. Resolution falls through to the VCS revision above.\n")
	} else {
		fmt.Fprintf(out, "- `CHRONICLE_VERSION`: `%s`\n", envVersion)
	}
	// Call the endpoint's own rule rather than restating it — a restated copy
	// is a copy that can disagree with the endpoint it claims to describe.
	fmt.Fprintf(out, "- `GET /api/version` therefore reports: **%s** (precedence: CHRONICLE_VERSION → compiled-in VCS revision → main module version → the literal `unknown`).\n\n",
		hostinfo.VersionFrom(envVersion, b))
}

// renderBuildExecutable prints the file this process is executing. This is the
// always-available fallback: it needs no build-time cooperation, so it survives
// the missing-git problem entirely, and its mtime is what actually settled the
// 2026-08-11 incident.
func renderBuildExecutable(out *strings.Builder, exe hostinfo.Executable, now time.Time) {
	out.WriteString("### Running executable — always available, and the closest honest proxy for build time\n\n")
	if exe.Path != "" {
		fmt.Fprintf(out, "- path: `%s`\n", exe.Path)
	}
	if exe.Err != "" {
		fmt.Fprintf(out, "- **could not be inspected**: %s\n", exe.Err)
		out.WriteString("- (size and mtime are unknown, not zero.)\n\n")
		return
	}
	fmt.Fprintf(out, "- size: %d bytes (%s)\n", exe.Size, humanBytes(exe.Size))
	fmt.Fprintf(out, "- mtime: %s (%s)\n", exe.ModTime.UTC().Format(time.RFC3339), relativeTo(exe.ModTime, now))
	out.WriteString("- The mtime is when this exact file was written — for a container, when the builder produced it (`COPY` preserves it). It belongs to the file this process is executing, which is more than any image label can claim.\n\n")
}

// renderBuildToolchain prints the Go build environment and module identity.
func renderBuildToolchain(out *strings.Builder, b hostinfo.Build) {
	out.WriteString("### Go toolchain and module\n\n")
	if !b.InfoOK {
		out.WriteString("- _unavailable — `debug.ReadBuildInfo()` returned nothing._\n\n")
		return
	}
	fmt.Fprintf(out, "- go version: %s\n", orUnrecorded(b.GoVersion))
	fmt.Fprintf(out, "- GOOS/GOARCH: %s/%s _(compiled-in; runtime reports %s/%s)_\n",
		orUnrecorded(b.GOOS), orUnrecorded(b.GOARCH), runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(out, "- main module: `%s`\n", orUnrecorded(b.MainPath))
	if b.MainVersion == "(devel)" {
		out.WriteString("- main module version: `(devel)` — the toolchain's own sentinel for \"no version information\", i.e. the same absence the revision section reports.\n\n")
		return
	}
	fmt.Fprintf(out, "- main module version: `%s`\n\n", orUnrecorded(b.MainVersion))
}

// renderBuildProcess prints process identity and uptime — the two facts that
// distinguish "this container was restarted with a new image five minutes ago"
// from "this container has been up since February".
func renderBuildProcess(out *strings.Builder, proc hostProcess, now time.Time) {
	out.WriteString("### This process\n\n")
	// A zero start time would otherwise render as "0001-01-01" and an uptime of
	// 2562047h — a nonsense number that looks like data. It cannot happen while
	// package init runs, but printing garbage confidently is the exact failure
	// this diagnostic exists to prevent, so it is handled rather than assumed.
	if proc.Start.IsZero() {
		out.WriteString("- started: _unknown_ (process start was never recorded)\n")
		out.WriteString("- uptime: _unknown_\n")
	} else {
		fmt.Fprintf(out, "- started: %s (%s)\n", proc.Start.UTC().Format(time.RFC3339), relativeTo(proc.Start, now))
		fmt.Fprintf(out, "- uptime: %s\n", shortDuration(now.Sub(proc.Start)))
	}
	fmt.Fprintf(out, "- pid: %d\n", proc.PID)
	if proc.HostnameErr != "" {
		fmt.Fprintf(out, "- hostname: _could not be determined_ (%s)\n", proc.HostnameErr)
	} else {
		fmt.Fprintf(out, "- hostname: `%s` _(inside a container this is the short container id unless overridden)_\n", proc.Hostname)
	}
	out.WriteString("- Start time is captured at Go package init, which runs before `main()` — within milliseconds of true process start, not identical to it.\n\n")
}

// renderBuildTrustNote is the payload of this whole diagnostic: the next reader
// will have a `docker inspect` window open, and without this note will reach
// the same wrong conclusion the 2026-08-11 investigation did.
func renderBuildTrustNote(out *strings.Builder) {
	out.WriteString("### Which of these to trust\n\n")
	out.WriteString("- **Statements this process makes about itself — trustworthy.** Executable path/size/mtime, Go version, GOOS/GOARCH, pid, hostname, start time and uptime. They are read from inside the running process; nothing can have relabelled them.\n")
	out.WriteString("- **Trustworthy when present, meaningless when absent — the VCS stamps.** Compiled in by the toolchain, so they cannot drift; but silently omitted when the build ran outside a checkout or without a VCS tool on PATH. Read \"not stamped\" as \"never recorded\", never as \"old\".\n")
	out.WriteString("- **NOT evidence about this process — Docker image labels.** `org.opencontainers.image.revision` and `.created` from `docker inspect <tag>` describe whichever image holds that tag *now*, which need not be the image this container was created from. On 2026-08-11 those labels read `revision=33f4cb07` / `created=2026-02-19` and were used to declare this binary six months stale; the binary had been built minutes earlier, and the labels were an accurate description of a February image still sitting in the deploy host's local image store. Chronicle's own `Dockerfile` now declares an `org.opencontainers.image.*` floor whose `revision` / `created` / `version` come from build args that **default to empty**, and CI overwrites them with real values. So a label here may describe a CI build, or be blank because the image was built locally — and a blank or stale label is still a statement about an IMAGE, never about this process.\n")
	out.WriteString("- **If you must correlate with Docker**, compare the image the container actually runs against the image the tag currently points at — if these differ, the labels are describing something else:\n")
	out.WriteString("\n```\ndocker inspect --format '{{.Image}}' <chronicle>\ndocker image inspect --format '{{.Id}} {{index .Config.Labels \"org.opencontainers.image.revision\"}}' ghcr.io/<org>/chronicle:latest\n```\n\n")
	out.WriteString("- **A related trap from the same incident:** grepping `/app/static` for a plugin's assets proves nothing. Plugin assets are `//go:embed`-ed into the binary and served from there, so they are absent from the on-disk static root by design.\n")
}

// hostIdentityLines is the COMPACT three-line build identity, for callers that
// need the answer without host.build's full account (today: host.deploy-check).
//
// It lives beside host.build's long renderer and shares notStampedHeadline with
// it deliberately. The alternative — a composite assembling its own summary
// from hostinfo — is a second opinion about what an absent VCS stamp means, and
// the one thing this whole workstream exists to prevent is two parts of
// Chronicle disagreeing about the identity of the binary they are both running
// inside.
func hostIdentityLines(b hostinfo.Build, exe hostinfo.Executable, proc hostProcess, now time.Time) []string {
	lines := make([]string, 0, 3)

	switch {
	case !b.InfoOK:
		lines = append(lines, "source revision: "+notStampedHeadline+" `debug.ReadBuildInfo()` returned nothing, so this binary was not produced by a module-aware Go build.")
	case !b.Stamped:
		lines = append(lines, "source revision: "+notStampedHeadline+" The toolchain stamps only when the build ran inside a checkout with a VCS tool on PATH, and skips it **silently** otherwise — read this as \"never recorded\", never as \"old\". Chronicle's current Dockerfile does stamp, so an unstamped binary here is most likely an image built before that change. `host.build` lists the other causes.")
	default:
		rev := "`" + b.ShortRevision() + "`"
		if b.ModifiedKnown && b.Modified {
			rev += " **built from a DIRTY tree** — the revision does not fully describe this binary"
		}
		if b.RevisionTime != "" {
			rev += " · committed " + b.RevisionTime
		}
		lines = append(lines, "source revision: "+rev)
	}

	switch {
	case exe.Err != "":
		lines = append(lines, fmt.Sprintf("executable: `%s` — **could not be inspected** (%s), so there is no build time to compare a deploy against.", exe.Path, exe.Err))
	case exe.Path == "" || exe.ModTime.IsZero():
		// Belt and braces: ReadExecutable always sets Err on failure, so this
		// is unreachable in a live process. It is handled anyway because the
		// alternative render is "0 B, written 0001-01-01T00:00:00Z" — a
		// fabricated field wearing the format of a fact, which is the exact
		// class of statement this entire workstream exists to eliminate.
		lines = append(lines, "executable: _not identified_ — no path or modification time was recorded, and no error explaining why. There is no build time here to compare a deploy against.")
	default:
		lines = append(lines, fmt.Sprintf("executable: `%s` — %s, written %s (%s). **This mtime is the strongest evidence of build recency available**: it belongs to the file this process is executing.",
			exe.Path, humanBytes(exe.Size), exe.ModTime.UTC().Format(time.RFC3339), relativeTo(exe.ModTime, now)))
	}

	if proc.Start.IsZero() {
		lines = append(lines, fmt.Sprintf("process: pid %d, start time was never recorded, so uptime is unknown.", proc.PID))
		return lines
	}
	lines = append(lines, fmt.Sprintf("process: up %s (started %s), pid %d. **Uptime longer than the time since your deploy means this container was never restarted** — a new image on the host changes nothing until something recreates the container.",
		shortDuration(now.Sub(proc.Start)), proc.Start.UTC().Format(time.RFC3339), proc.PID))
	return lines
}

// renderHostRuntime samples the live runtime. runtime.ReadMemStats briefly
// stops the world, which is why this is a named diagnostic an operator runs on
// purpose rather than something rendered on every admin page.
func renderHostRuntime() string {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return renderHostRuntimeFrom(&ms, runtimeCounts{
		Goroutines: runtime.NumGoroutine(),
		NumCPU:     runtime.NumCPU(),
		// GOMAXPROCS(0) queries without changing anything.
		GOMAXPROCS: runtime.GOMAXPROCS(0),
	}, hostinfo.ProcessStart(), time.Now())
}

// runtimeCounts is the non-memory half of the runtime snapshot, injected so the
// renderer stays pure and testable.
type runtimeCounts struct {
	Goroutines int
	NumCPU     int
	GOMAXPROCS int
}

// renderHostRuntimeFrom is the pure renderer for host.runtime.
func renderHostRuntimeFrom(ms *runtime.MemStats, c runtimeCounts, start, now time.Time) string {
	var out strings.Builder
	out.WriteString("## host.runtime — process runtime snapshot\n\n")

	fmt.Fprintf(&out, "- uptime: **%s** (started %s)\n", shortDuration(now.Sub(start)), start.UTC().Format(time.RFC3339))
	fmt.Fprintf(&out, "- goroutines: **%d**\n", c.Goroutines)
	fmt.Fprintf(&out, "- NumCPU: %d  ·  GOMAXPROCS: %d\n", c.NumCPU, c.GOMAXPROCS)
	if c.GOMAXPROCS < c.NumCPU {
		out.WriteString("  - _GOMAXPROCS is below NumCPU — either a container CPU limit was applied, or GOMAXPROCS was set explicitly._\n")
	}
	out.WriteString("\n### Memory\n\n")
	fmt.Fprintf(&out, "- Alloc (live heap): %s\n", humanBytes(int64(ms.Alloc)))
	fmt.Fprintf(&out, "- HeapInuse (heap spans in use): %s\n", humanBytes(int64(ms.HeapInuse)))
	fmt.Fprintf(&out, "- Sys (total obtained from the OS): %s\n", humanBytes(int64(ms.Sys)))
	fmt.Fprintf(&out, "- HeapObjects: %d  ·  TotalAlloc (cumulative, never decreases): %s\n", ms.HeapObjects, humanBytes(int64(ms.TotalAlloc)))
	out.WriteString("- Compare against the container's memory limit, not against the host's RAM. `Sys` is what the process took from the OS; `Alloc` is what is live right now, and the gap between them is returned-but-unmapped memory, not a leak.\n")

	out.WriteString("\n### Garbage collection\n\n")
	fmt.Fprintf(&out, "- completed cycles: %d\n", ms.NumGC)
	if ms.NumGC == 0 {
		out.WriteString("- no GC has run yet in this process.\n")
		return out.String()
	}
	lastGC := time.Unix(0, int64(ms.LastGC))
	fmt.Fprintf(&out, "- last GC: %s (%s)\n", lastGC.UTC().Format(time.RFC3339), relativeTo(lastGC, now))
	fmt.Fprintf(&out, "- total STW pause: %s  ·  mean per cycle: %s\n",
		shortNanos(ms.PauseTotalNs), shortNanos(ms.PauseTotalNs/uint64(ms.NumGC)))
	fmt.Fprintf(&out, "- GC CPU fraction: %.4f%%\n", ms.GCCPUFraction*100)
	fmt.Fprintf(&out, "- recent pauses (newest first): %s\n", recentPauses(ms, 5))
	out.WriteString("- Pauses are stop-the-world time. Sustained multi-millisecond pauses or a rising GC CPU fraction point at allocation pressure, not at a wedged process.\n")

	return out.String()
}

// recentPauses reads the newest n entries out of the runtime's 256-slot
// circular pause buffer. The ring is indexed by (NumGC+255)%256 for the most
// recent entry, and walking it backwards is the only way to get pause history
// without instrumenting GC ourselves.
func recentPauses(ms *runtime.MemStats, n int) string {
	if ms.NumGC == 0 {
		return "_none_"
	}
	if uint32(n) > ms.NumGC {
		n = int(ms.NumGC)
	}
	if n > len(ms.PauseNs) {
		n = len(ms.PauseNs)
	}
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		idx := (int(ms.NumGC) - 1 - i + len(ms.PauseNs)*2) % len(ms.PauseNs)
		parts = append(parts, shortNanos(ms.PauseNs[idx]))
	}
	return strings.Join(parts, ", ")
}

// shortNanos renders a nanosecond count at a granularity a human can compare
// (µs below a millisecond, then ms/s via Duration's own formatting).
func shortNanos(ns uint64) string {
	d := time.Duration(ns)
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	return d.Round(time.Microsecond).String()
}

// orUnrecorded keeps an absent value from rendering as an empty pair of
// backticks, which reads as a bug rather than as a gap.
func orUnrecorded(s string) string {
	if strings.TrimSpace(s) == "" {
		return "_not recorded_"
	}
	return s
}

// relativeTo renders "3h12m ago" / "in the future". A timestamp in the future
// is called out rather than printed as a negative age, because it means clock
// skew between the container and the host — itself a finding.
func relativeTo(t, now time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := now.Sub(t)
	if d < 0 {
		return shortDuration(-d) + " in the FUTURE — clock skew?"
	}
	return shortDuration(d) + " ago"
}

// shortDuration renders a duration at second granularity; sub-second noise in
// an uptime or a file age is never the answer to anything.
func shortDuration(d time.Duration) string {
	if d < time.Second {
		return "under 1s"
	}
	return d.Truncate(time.Second).String()
}
