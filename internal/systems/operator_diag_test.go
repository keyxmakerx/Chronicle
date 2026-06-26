package systems

import (
	"strings"
	"testing"
)

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
