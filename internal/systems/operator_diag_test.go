package systems

import (
	"strings"
	"testing"
	"time"
)

func TestCatalogHasNewDiagnostics(t *testing.T) {
	names := map[string]bool{}
	for _, d := range diagnosticCatalog() {
		names[d.Name] = true
	}
	for _, want := range []string{"packages.installed-vs-loaded", "packages.on-disk-versions", "systems.load-events"} {
		if !names[want] {
			t.Errorf("catalog missing %q", want)
		}
	}
}

func TestRenderLoadEvents(t *testing.T) {
	if !strings.Contains(renderLoadEvents(nil), "No load events") {
		t.Error("empty events should render the no-events note")
	}
	evs := []LoadEvent{
		{Timestamp: time.Unix(1000, 0), SystemID: "drawsteel", Kind: EventSkipped, Source: "package", Error: "dup ignored", Dir: "/p/drawsteel/0.12.3"},
		{Timestamp: time.Unix(2000, 0), SystemID: "drawsteel", Kind: EventDiscovered, Source: "package", Dir: "/p/drawsteel/0.13.0"},
	}
	out := renderLoadEvents(evs)
	if !strings.Contains(out, "skipped") || !strings.Contains(out, "dup ignored") {
		t.Errorf("expected the skipped event + its reason, got:\n%s", out)
	}
}

func TestInstalledVsLoaded(t *testing.T) {
	prev := installedPackagesFn
	defer func() { installedPackagesFn = prev }()

	installedPackagesFn = nil
	if !strings.Contains(renderInstalledVsLoaded(), "Provider not wired") {
		t.Error("nil provider should be reported, not crash")
	}

	// Installed 0.13.0 but the loader serves the 0.12.3 dir → "NOT loaded".
	stale := map[string]*loadedSystem{
		"drawsteel": {manifest: &SystemManifest{ID: "drawsteel", Name: "Draw Steel", Version: "0.12.3"}, dir: "/p/drawsteel/0.12.3", source: "package"},
	}
	withLoadedSystems(t, stale, func() {
		installedPackagesFn = func() []InstalledPackage {
			return []InstalledPackage{{Slug: "Chronicle-Draw-Steel", Version: "0.13.0", InstallPath: "/p/drawsteel/0.13.0"}}
		}
		out := renderInstalledVsLoaded()
		if !strings.Contains(out, "NOT loaded") {
			t.Errorf("stale loader should report NOT loaded, got:\n%s", out)
		}
	})

	// One matched-and-equal (OK), one matched-but-different-version (MISMATCH).
	mods := map[string]*loadedSystem{
		"a": {manifest: &SystemManifest{ID: "a", Version: "0.13.0"}, dir: "/p/a/0.13.0", source: "package"},
		"b": {manifest: &SystemManifest{ID: "b", Version: "0.12.0"}, dir: "/p/b/0.13.0", source: "package"},
	}
	withLoadedSystems(t, mods, func() {
		installedPackagesFn = func() []InstalledPackage {
			return []InstalledPackage{
				{Slug: "a", Version: "0.13.0", InstallPath: "/p/a/0.13.0"},
				{Slug: "b", Version: "0.13.0", InstallPath: "/p/b/0.13.0"},
			}
		}
		out := renderInstalledVsLoaded()
		if !strings.Contains(out, "MISMATCH") {
			t.Errorf("b (loaded 0.12.0 vs installed 0.13.0) should be MISMATCH, got:\n%s", out)
		}
	})
}

func TestDiagnosticCatalog_WellFormed(t *testing.T) {
	cat := diagnosticCatalog()
	if len(cat) == 0 {
		t.Fatal("expected a non-empty diagnostic catalog")
	}
	seen := map[string]bool{}
	for _, d := range cat {
		if d.Name == "" || d.Title == "" || d.Desc == "" || d.Run == nil {
			t.Errorf("diagnostic %q has an empty field: %+v", d.Name, d)
		}
		if seen[d.Name] {
			t.Errorf("duplicate diagnostic name %q", d.Name)
		}
		seen[d.Name] = true
	}
	// The catalog the assistant reads must name every diagnostic but carry no
	// payload data (no garbage context).
	menu := renderCatalog(cat)
	for _, d := range cat {
		if !strings.Contains(menu, d.Name) {
			t.Errorf("catalog menu missing %q", d.Name)
		}
	}
}

func TestRunDiagnostic_DispatchAndUnknown(t *testing.T) {
	cat := diagnosticCatalog()
	if _, ok := RunDiagnostic(cat, "does.not.exist", ""); ok {
		t.Error("unknown diagnostic should return ok=false")
	}
	out, ok := RunDiagnostic(cat, "probes", "")
	if !ok {
		t.Fatal("probes diagnostic should dispatch")
	}
	// Probes diagnostic carries the run-and-paste-back library.
	if !strings.Contains(out, "PASTE OUTPUT BELOW") {
		t.Error("probes output should include paste-back markers")
	}
	for _, p := range defaultProbes() {
		if !strings.Contains(out, p.Command) {
			t.Errorf("probes output missing command for %q", p.ID)
		}
	}
}

func TestProbes_WellFormedAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range defaultProbes() {
		if p.ID == "" || p.Title == "" || p.Command == "" || p.Why == "" || p.Where == "" {
			t.Errorf("probe %q has an empty field: %+v", p.ID, p)
		}
		if seen[p.ID] {
			t.Errorf("duplicate probe id %q", p.ID)
		}
		seen[p.ID] = true
	}
}

func TestRedactSecrets(t *testing.T) {
	cases := []struct {
		in       string
		redacted bool
	}{
		{"DB_PASSWORD=hunter2", true},
		{"api_key: sk-abcdef123456", true},
		{"Authorization: Bearer eyJhbGciOi", true},
		{"Bearer abc.def.ghi", true}, // space-separated bearer (no [:=] separator)
		{"private-key = MIIEvA", true},
		// must NOT redact legitimate diagnostic data (no secret keyword):
		{"sha256: deadbeefcafe1234", false},
		{"loaded_version: 0.13.0", false},
		{"dir: /app/media/packages/systems/drawsteel/0.13.0", false},
	}
	for _, c := range cases {
		got := redactSecrets(c.in)
		didRedact := strings.Contains(got, "[REDACTED]")
		if didRedact != c.redacted {
			t.Errorf("redactSecrets(%q) redacted=%v, want %v (got %q)", c.in, didRedact, c.redacted, got)
		}
	}
}

func TestRunDiagnostic_OutputIsRedacted(t *testing.T) {
	// A diagnostic whose raw output contains a credential must come back redacted.
	cat := []Diagnostic{{
		Name: "leaky", Title: "t", Desc: "d",
		Run: func(string) string { return "config: DB_PASSWORD=hunter2\n" },
	}}
	out, ok := RunDiagnostic(cat, "leaky", "")
	if !ok {
		t.Fatal("expected dispatch")
	}
	if strings.Contains(out, "hunter2") || !strings.Contains(out, "[REDACTED]") {
		t.Errorf("expected redacted output, got %q", out)
	}
}
