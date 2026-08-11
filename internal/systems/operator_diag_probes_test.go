package systems

import (
	"strings"
	"testing"
)

// The probe library is the only part of the diagnostics that an operator runs
// with their own hands, unsupervised, from memory. That makes its PROSE the
// deliverable: a command whose output is truthful about what it measured and
// misleading about what the reader wanted to know is worse than no command, and
// both wrong turns of the 2026-08-11 incident were exactly that. These tests
// pin the warnings, not the commands.

// probeByID is a small lookup so a failure names the probe rather than an index.
func probeByID(t *testing.T, id string) Probe {
	t.Helper()
	for _, p := range defaultProbes() {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("probe %q is not in defaultProbes()", id)
	return Probe{}
}

// TestProbeLabelTrapIsAnnotated pins the first wrong turn. The image-digest
// probe is the one an operator reaches for when asking "which build is this?",
// and its answer was read as evidence about a running binary that it cannot
// provide. The warning has to be attached to the probe itself, because the
// operator reading it will have a `docker inspect` window already open.
func TestProbeLabelTrapIsAnnotated(t *testing.T) {
	p := probeByID(t, "image-digest")

	if !strings.Contains(p.Title, "TRAP") {
		t.Errorf("the image probe must be marked as a trap in its title; got %q", p.Title)
	}
	for _, want := range []string{
		"NOT a reliable build identity",
		"six months stale",
		"host.build",
	} {
		if !strings.Contains(p.Why, want) {
			t.Errorf("image-digest's Why must contain %q; got:\n%s", want, p.Why)
		}
	}
	// The whole point of the upgraded command is the COMPARISON: the image the
	// container runs versus the image the tag points at now. One of those alone
	// is what produced the retracted conclusion.
	if !strings.Contains(p.Command, "{{.Image}}") || !strings.Contains(p.Command, "docker image inspect") {
		t.Errorf("image-digest must compare the container's image against the tag's image; got:\n%s", p.Command)
	}
}

// TestProbeEmbeddedAssetTrapIsAnnotated pins the second wrong turn: a grep of
// the container filesystem for a plugin's asset returns nothing BY DESIGN, and
// an operator who does not know that reads it as missing code.
func TestProbeEmbeddedAssetTrapIsAnnotated(t *testing.T) {
	p := probeByID(t, "plugin-asset-grep")

	if !strings.Contains(p.Title, "TRAP") {
		t.Errorf("the filesystem-grep probe must be marked as a trap in its title; got %q", p.Title)
	}
	for _, want := range []string{
		"EXPECTED result",
		"not evidence of missing code",
		"host.embedded",
	} {
		if !strings.Contains(p.Why, want) {
			t.Errorf("plugin-asset-grep's Why must contain %q; got:\n%s", want, p.Why)
		}
	}
}

// TestSupersededProbesAreAnnotatedNotDeleted pins the brief's rule literally:
// a probe a host.* diagnostic now answers better keeps its entry and says so.
// Deleting it would leave an operator who remembers the command running it with
// no note attached — the worst of both worlds.
func TestSupersededProbesAreAnnotatedNotDeleted(t *testing.T) {
	cases := []struct {
		id      string
		mustRef string
	}{
		{"package-version-dirs", "packages.on-disk-versions"},
		{"package-file-marker", "system.file-contains"},
		{"chronicle-logs", "host.errors"},
		{"served-widget-version", "host.widgets"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			p := probeByID(t, tc.id)
			if !strings.Contains(p.Why, tc.mustRef) {
				t.Errorf("%s must point at %s; got:\n%s", tc.id, tc.mustRef, p.Why)
			}
		})
	}

	// The pre-existing library must not have shrunk. A probe silently removed
	// is the failure mode this test exists to catch.
	have := map[string]bool{}
	for _, p := range defaultProbes() {
		have[p.ID] = true
	}
	for _, id := range []string{
		"served-widget-version", "served-widget-content", "package-version-dirs",
		"package-file-marker", "chronicle-logs", "image-digest",
		"packages-db-state", "entity-type-tree", "sync-mapping-orphans",
	} {
		if !have[id] {
			t.Errorf("probe %q was removed; annotate it instead of deleting it", id)
		}
	}
}

// TestNewIncidentProbesExist pins the probes the incident proved were needed:
// was the container ever recreated, and what does the binary's own mtime look
// like from outside the process.
func TestNewIncidentProbesExist(t *testing.T) {
	restart := probeByID(t, "container-restart-time")
	if !strings.Contains(restart.Command, "StartedAt") {
		t.Errorf("the restart probe must read the container's StartedAt; got:\n%s", restart.Command)
	}
	if !strings.Contains(restart.Why, "neither rebuilds nor re-pulls") {
		t.Errorf("the restart probe must explain why `up -d` can be a silent no-op; got:\n%s", restart.Why)
	}

	bin := probeByID(t, "binary-in-container")
	if !strings.Contains(bin.Command, "/usr/local/bin/chronicle") {
		t.Errorf("the binary probe must point at the path the Dockerfile installs to; got:\n%s", bin.Command)
	}
	if !strings.Contains(bin.Why, "date -u") && !strings.Contains(bin.Why, "container clock") {
		t.Errorf("the binary probe must explain the clock check; got:\n%s", bin.Why)
	}

	schema := probeByID(t, "plugin-schema-versions")
	if !strings.Contains(schema.Command, "plugin_schema_versions") {
		t.Errorf("the schema probe must query plugin_schema_versions; got:\n%s", schema.Command)
	}
	if !strings.Contains(schema.Why, "host.plugins") {
		t.Errorf("the schema probe must relate itself to host.plugins; got:\n%s", schema.Why)
	}
}

// TestProbesSectionTellsTheReaderToReadTheWhyFirst pins the header. The Why is
// where the warnings live, and a reader who scrolls straight to the command
// block gets no benefit from any of the annotations above.
func TestProbesSectionTellsTheReaderToReadTheWhyFirst(t *testing.T) {
	var b strings.Builder
	renderProbesSection(&b, defaultProbes())
	out := b.String()

	if !strings.Contains(out, "Read each `Why:` before running") {
		t.Errorf("the probes header must tell the reader to read the Why first:\n%s", out)
	}
	if !strings.Contains(out, "TRAP") {
		t.Errorf("the rendered probes section must carry the trap markers:\n%s", out)
	}
	if !strings.Contains(out, "<org>") {
		t.Errorf("the placeholder list must cover <org>, which the image probe now uses:\n%s", out)
	}
	for _, p := range defaultProbes() {
		if !strings.Contains(out, p.Command) {
			t.Errorf("rendered section is missing the command for %q", p.ID)
		}
		if !strings.Contains(out, p.Why) {
			t.Errorf("rendered section is missing the Why for %q — the annotation is the payload", p.ID)
		}
	}
}

// TestProbePlaceholdersAreDocumented pins that every <placeholder> a probe uses
// is named in the section header. An undocumented placeholder is a command the
// operator cannot run.
func TestProbePlaceholdersAreDocumented(t *testing.T) {
	var b strings.Builder
	renderProbesSection(&b, defaultProbes())
	// Only the preamble counts. Comparing against the whole rendered section
	// would be vacuous — every placeholder appears there inside the very
	// command that needs it explained.
	header, _, _ := strings.Cut(b.String(), "### ")

	for _, p := range defaultProbes() {
		for _, ph := range []string{"<chronicle>", "<db>", "<media>", "<campaignId>", "<org>", "<password>"} {
			if strings.Contains(p.Command, ph) && !strings.Contains(header, ph) {
				t.Errorf("probe %q uses %s but the header does not explain it", p.ID, ph)
			}
		}
	}
}
