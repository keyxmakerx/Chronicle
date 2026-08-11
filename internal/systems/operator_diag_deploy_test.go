package systems

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/keyxmakerx/chronicle/internal/hostinfo"
)

// host.deploy-check exists because four separate diagnostics already held the
// answer to "did my deploy land?" and nobody assembled them. These tests pin
// the two things a composite can get wrong: restating a delegate's logic
// instead of calling it (a second opinion that can disagree), and flattening
// "not scanned" into "not found" (the absence-of-evidence error that started
// all of this).

// deployTestRoot builds a static root with the bellwether files plus a marker
// that exists ONLY on disk.
func deployTestRoot(t *testing.T) staticRoot {
	t.Helper()
	root := t.TempDir()
	for rel, body := range map[string]string{
		"css/app.css":  ".tw{--disk-only-marker:1}",
		"js/boot.js":   "// boot",
		"js/theme.js":  "// theme",
		"img/logo.png": "\x89PNG not text",
	} {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return staticRoot{CWD: filepath.Dir(root), Path: root, Exists: true}
}

// embeddedOnlySet holds a marker that exists ONLY inside the binary — the
// situation a filesystem grep reports as missing code.
func embeddedOnlySet() []EmbeddedAssetSet {
	return []EmbeddedAssetSet{{
		Slug:      "calendar",
		URLPrefix: "/static/plugins/calendar/",
		FS: fstest.MapFS{
			"js/gm_panel.js":   &fstest.MapFile{Data: []byte("function moonPhase(){}")},
			"css/gm_panel.css": &fstest.MapFile{Data: []byte(".gm{}")},
		},
	}}
}

func deploySources(t *testing.T) deployCheckSources {
	t.Helper()
	return deployCheckSources{
		Root:       deployTestRoot(t),
		Embedded:   embeddedOnlySet,
		AssetURL:   func(u string) string { return u + "?v=deadbeef01" },
		BuildToken: "buildtok01",
		Packages: func() string {
			return "## packages.installed-vs-loaded\n\n- `drawsteel` installed **0.13.0** · loaded **0.13.0**"
		},
		Now: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
	}
}

// TestDeployCheckIsRegistered pins the catalog entry and the claims its Desc
// makes — it is the entry an operator is told to run after a deploy, so it has
// to advertise the optional marker argument and the two-scope search.
func TestDeployCheckIsRegistered(t *testing.T) {
	var d *Diagnostic
	for _, c := range diagnosticCatalog() {
		if c.Name == "host.deploy-check" {
			cc := c
			d = &cc
		}
	}
	if d == nil {
		t.Fatal("host.deploy-check is not registered in diagnosticCatalog()")
	}
	if d.ArgHint == "" {
		t.Error("host.deploy-check takes an optional marker list and must advertise it")
	}
	if d.FullDump {
		t.Error("host.deploy-check is the 'paste this one thing after a deploy' check; gating it behind full_dump defeats its purpose")
	}
	if !strings.Contains(d.Desc, "embedded") {
		t.Errorf("the Desc must say the marker search covers the embedded assets too; got: %s", d.Desc)
	}
}

// TestDeployCheckMarkerFoundOnlyInEmbeddedIsNotMissing is THE test. A marker
// present only inside the binary must be reported as found-in-the-binary and
// explicitly not-missing — reading that case as absent code is what cost an
// hour on 2026-08-11.
func TestDeployCheckMarkerFoundOnlyInEmbeddedIsNotMissing(t *testing.T) {
	out := renderHostDeployCheckFrom(deploySources(t), "moonPhase")

	if !strings.Contains(out, "✗ on-disk static root: not found") {
		t.Errorf("the on-disk scope must report the marker absent:\n%s", out)
	}
	if !strings.Contains(out, "✓ embedded in the binary: found in 1 file(s) — `calendar:js/gm_panel.js`") {
		t.Errorf("the embedded scope must report the marker found, labelled with its plugin:\n%s", out)
	}
	if !strings.Contains(out, "found only in the embedded scope is not missing") {
		t.Errorf("the output must state that this exact combination is not a missing feature:\n%s", out)
	}
}

// TestDeployCheckMarkerScopesAreReportedSeparately pins the inverse case and
// the both-absent case, so the three outcomes cannot collapse into one verdict.
func TestDeployCheckMarkerScopesAreReportedSeparately(t *testing.T) {
	out := renderHostDeployCheckFrom(deploySources(t), "--disk-only-marker,neither-place")

	if !strings.Contains(out, "✓ on-disk static root: found in 1 file(s) — `css/app.css`") {
		t.Errorf("a disk-only marker must be found on disk:\n%s", out)
	}
	if !strings.Contains(out, "**`neither-place`**") {
		t.Errorf("every requested marker must get a line:\n%s", out)
	}
	if !strings.Contains(out, "absent from BOTH scopes") {
		t.Errorf("the output must say what absence from both scopes means:\n%s", out)
	}
}

// TestDeployCheckUnscannedScopeIsNotAbsence pins the distinction the whole
// workstream turns on: a scope that could not be read must say "not scanned",
// never "not found".
func TestDeployCheckUnscannedScopeIsNotAbsence(t *testing.T) {
	src := deploySources(t)
	src.Embedded = nil // the provider is unwired
	src.Root = staticRoot{CWD: "/x", Path: "/x/static", StatErr: "no such file or directory"}

	out := renderHostDeployCheckFrom(src, "moonPhase")
	if strings.Count(out, "_not scanned_") != 2 {
		t.Errorf("both unreadable scopes must report 'not scanned', got:\n%s", out)
	}
	if strings.Contains(out, "not found") {
		t.Errorf("nothing was scanned, so nothing may be reported as not found:\n%s", out)
	}
	if !strings.Contains(out, "this is not \"absent\"") {
		t.Errorf("the not-scanned line must deny the 'absent' reading:\n%s", out)
	}
	if !strings.Contains(out, "not \"the marker is absent from the binary\"") {
		t.Errorf("the unwired embedded provider must deny the 'absent from the binary' reading:\n%s", out)
	}
}

// TestDeployCheckWiredButEmptyEmbeddedIsAnAnswer separates "no plugin embeds
// assets in this build" from "nobody wired the provider".
func TestDeployCheckWiredButEmptyEmbeddedIsAnAnswer(t *testing.T) {
	src := deploySources(t)
	src.Embedded = func() []EmbeddedAssetSet { return nil }
	out := renderHostDeployCheckFrom(src, "moonPhase")
	if !strings.Contains(out, "IS an answer") {
		t.Errorf("a wired-but-empty embedded scope must say its emptiness is an answer:\n%s", out)
	}
	if !strings.Contains(out, "✗ embedded in the binary: not found") {
		t.Errorf("a wired-but-empty scope WAS scanned, so 'not found' is correct here:\n%s", out)
	}
}

// TestDeployCheckWithoutMarkersStillAnswersTheRest pins that the argument is
// genuinely optional — sections 1, 2 and 4 are useful on their own, and the
// marker section explains what to pass rather than rendering blank.
func TestDeployCheckWithoutMarkersStillAnswersTheRest(t *testing.T) {
	out := renderHostDeployCheckFrom(deploySources(t), "")
	for _, want := range []string{
		"1. Which binary is running",
		"2. The assets that move on almost every build",
		"No marker given, so nothing was searched",
		"host.deploy-check moonPhase",
		"4. Installed vs loaded system packages",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// TestDeployCheckDelegatesRatherThanRestates pins the anti-duplication rule: the
// package summary must be the existing diagnostic's own output, verbatim. A
// composite that reimplemented it would be a second opinion that can disagree
// with the check it claims to summarise — while someone is using it to decide
// whether a deploy landed.
func TestDeployCheckDelegatesRatherThanRestates(t *testing.T) {
	src := deploySources(t)
	sentinel := "## packages.installed-vs-loaded\n\n- SENTINEL-FROM-THE-DELEGATE"
	src.Packages = func() string { return sentinel }
	out := renderHostDeployCheckFrom(src, "")
	if !strings.Contains(out, "SENTINEL-FROM-THE-DELEGATE") {
		t.Errorf("the package section must be the delegate's output verbatim:\n%s", out)
	}

	src.Packages = nil
	if got := renderHostDeployCheckFrom(src, ""); !strings.Contains(got, "Package summary unavailable") {
		t.Errorf("a missing delegate must degrade to a note, not a panic:\n%s", got)
	}
}

// TestDeployCheckBellwethers pins that each named asset is reported with its
// reason, that a missing one is named rather than skipped, and that the newest
// mtime is surfaced — that line is what an operator compares against the clock
// time of their deploy.
func TestDeployCheckBellwethers(t *testing.T) {
	out := renderHostDeployCheckFrom(deploySources(t), "")
	for _, rel := range []string{"css/app.css", "js/boot.js", "js/theme.js"} {
		if !strings.Contains(out, "`"+rel+"`") {
			t.Errorf("bellwether %s is missing:\n%s", rel, out)
		}
	}
	if !strings.Contains(out, "Newest of these:") {
		t.Errorf("the newest bellwether mtime must be surfaced:\n%s", out)
	}

	// A named file that has gone missing is itself a finding and must be
	// printed, not silently dropped from the list.
	src := deploySources(t)
	if err := os.Remove(filepath.Join(src.Root.Path, "css", "app.css")); err != nil {
		t.Fatal(err)
	}
	missing := renderHostDeployCheckFrom(src, "")
	if !strings.Contains(missing, "`css/app.css` — **not present**") {
		t.Errorf("a missing bellwether must be named:\n%s", missing)
	}
	if !strings.Contains(missing, "GENERATED at build time") {
		t.Errorf("the missing-app.css line must explain that it is generated, so dev and prod read differently:\n%s", missing)
	}
}

// TestDeployCheckUnusableStaticRootExplainsTheDeploy pins that an unresolvable
// static root is reported as the explanation it is, rather than as an empty
// asset list.
func TestDeployCheckUnusableStaticRootExplainsTheDeploy(t *testing.T) {
	src := deploySources(t)
	src.Root = staticRoot{CWD: "/x", Path: "/x/static", StatErr: "no such file or directory"}
	out := renderHostDeployCheckFrom(src, "")
	if !strings.Contains(out, "explains a deploy that appears not to have landed") {
		t.Errorf("an unusable static root is itself the answer and must say so:\n%s", out)
	}
}

// TestDeployCheckAlwaysDeniesTheLabelEvidence pins the one sentence that would
// have prevented the retracted conclusion, on every run.
func TestDeployCheckAlwaysDeniesTheLabelEvidence(t *testing.T) {
	out := renderHostDeployCheckFrom(deploySources(t), "")
	if !strings.Contains(out, "Image labels are NOT part of this answer") {
		t.Errorf("every run must deny image labels as evidence:\n%s", out)
	}
	if !strings.Contains(out, "docker compose up -d") {
		t.Errorf("the remedy section must name the compose behaviour that produces a silent no-op deploy:\n%s", out)
	}
}

// TestHostIdentityLines drives the three-line summary through the states a real
// deployment can be in. The unstamped case is the one every shipped image is in
// today, and the zero-executable case is the one that would otherwise print
// "written 0001-01-01" as though it were a measurement.
func TestHostIdentityLines(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	proc := hostProcess{PID: 7, Start: now.Add(-90 * time.Minute), Hostname: "abc123"}

	unstamped := strings.Join(hostIdentityLines(hostinfo.Build{InfoOK: true}, hostinfo.Executable{Path: "/usr/local/bin/chronicle", Size: 100, ModTime: now.Add(-time.Hour)}, proc, now), "\n")
	if !strings.Contains(unstamped, notStampedHeadline) {
		t.Errorf("an unstamped build must use the shared not-stamped wording:\n%s", unstamped)
	}
	if !strings.Contains(unstamped, "never recorded") {
		t.Errorf("an unstamped build must be distinguished from an old one:\n%s", unstamped)
	}
	if !strings.Contains(unstamped, "1h0m0s ago") {
		t.Errorf("the executable age must be rendered:\n%s", unstamped)
	}
	if !strings.Contains(unstamped, "up 1h30m0s") {
		t.Errorf("uptime must be rendered:\n%s", unstamped)
	}
	if !strings.Contains(unstamped, "never restarted") {
		t.Errorf("the uptime line must explain what a long uptime means for a deploy:\n%s", unstamped)
	}

	stamped := strings.Join(hostIdentityLines(hostinfo.Build{
		InfoOK: true, Stamped: true, Revision: "84e31334d5b9ff78b940fe452efc502cbae8f707",
		RevisionTime: "2026-08-10T18:11:57Z", Modified: true, ModifiedKnown: true,
	}, hostinfo.Executable{Path: "/x", ModTime: now}, proc, now), "\n")
	if !strings.Contains(stamped, "84e31334d5b9") {
		t.Errorf("a stamped build must print the short revision:\n%s", stamped)
	}
	if !strings.Contains(stamped, "DIRTY") {
		t.Errorf("a dirty build tree must be called out:\n%s", stamped)
	}

	noInfo := strings.Join(hostIdentityLines(hostinfo.Build{}, hostinfo.Executable{}, hostProcess{}, now), "\n")
	if strings.Contains(noInfo, "0001-01-01") {
		t.Errorf("a zero executable must never render the zero time as data:\n%s", noInfo)
	}
	if !strings.Contains(noInfo, "not identified") {
		t.Errorf("a zero executable must say it was not identified:\n%s", noInfo)
	}
	if !strings.Contains(noInfo, "start time was never recorded") {
		t.Errorf("a zero process start must say so rather than claim a 2562047h uptime:\n%s", noInfo)
	}
}

// TestParseMarkers pins the argument handling: duplicates collapse, blanks are
// dropped, and an over-long list is clamped WITH a note naming what was
// dropped. A silently ignored marker would let someone conclude "absent" about
// a string that was never searched for.
func TestParseMarkers(t *testing.T) {
	got, note := parseMarkers(" a , b ,, a ")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("parseMarkers = %v, want [a b]", got)
	}
	if note != "" {
		t.Errorf("no note expected for a short list, got %q", note)
	}

	many := make([]string, 0, maxMarkers+3)
	for i := 0; i < maxMarkers+3; i++ {
		many = append(many, string(rune('a'+i)))
	}
	got, note = parseMarkers(strings.Join(many, ","))
	if len(got) != maxMarkers {
		t.Errorf("clamped list length = %d, want %d", len(got), maxMarkers)
	}
	if !strings.Contains(note, "Not searched") {
		t.Errorf("the clamp must name what it dropped, got %q", note)
	}

	if got, _ := parseMarkers("  "); len(got) != 0 {
		t.Errorf("a blank argument must yield no markers, got %v", got)
	}
}

// TestMarkerScanSkipsNonTextAndSaysSo pins that the extension bound is a stated
// fact with a number rather than a silent omission.
func TestMarkerScanSkipsNonTextAndSaysSo(t *testing.T) {
	hits, note := scanDiskForMarkers(deployTestRoot(t), []string{"--disk-only-marker"})
	if len(hits["--disk-only-marker"]) != 1 {
		t.Errorf("expected exactly one on-disk hit, got %v", hits)
	}
	if !strings.Contains(note, "skipped 1 non-text") {
		t.Errorf("the scan must report how many files the extension bound excluded, got %q", note)
	}
}
