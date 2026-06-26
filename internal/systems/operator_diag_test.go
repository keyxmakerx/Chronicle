package systems

import (
	"strings"
	"testing"
)

func TestDefaultProbes_WellFormed(t *testing.T) {
	probes := defaultProbes()
	if len(probes) == 0 {
		t.Fatal("expected a non-empty probe library")
	}
	seen := map[string]bool{}
	for _, p := range probes {
		if p.ID == "" || p.Title == "" || p.Command == "" || p.Why == "" || p.Where == "" {
			t.Errorf("probe %q has an empty field: %+v", p.ID, p)
		}
		if seen[p.ID] {
			t.Errorf("duplicate probe id %q", p.ID)
		}
		seen[p.ID] = true
	}
}

func TestBuildOperatorReport_EmbedsServedRealityAndProbes(t *testing.T) {
	systems := []SystemHealth{{
		ID: "drawsteel", Name: "Draw Steel", Version: "0.13.0", Source: "package",
		Dir: "/app/media/packages/systems/drawsteel/0.13.0",
		Files: []FileFingerprint{
			{Path: "widgets/character-sheet.js", Exists: true, Size: 41000, SHA256: "deadbeefcafe1234", ModTime: "2026-06-26T20:06:00Z"},
			{Path: "missing.js", Exists: false},
		},
	}}
	report := BuildOperatorReport(systems, defaultProbes())

	// Served reality must be present and specific (version, dir, hash, MISSING).
	for _, want := range []string{
		"Chronicle Operator Diagnostics",
		"no secrets included",
		"loaded_version: **0.13.0**",
		"/app/media/packages/systems/drawsteel/0.13.0",
		"deadbeefcafe1234",
		"**MISSING**",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q", want)
		}
	}

	// Every probe's command + a paste-back marker must appear.
	for _, p := range defaultProbes() {
		if !strings.Contains(report, p.Command) {
			t.Errorf("report missing probe command for %q", p.ID)
		}
	}
	if strings.Count(report, "PASTE OUTPUT BELOW") != len(defaultProbes()) {
		t.Errorf("expected one paste-back marker per probe (%d)", len(defaultProbes()))
	}
}

func TestBuildOperatorReport_EmptySystemsIsGraceful(t *testing.T) {
	report := BuildOperatorReport(nil, defaultProbes())
	if !strings.Contains(report, "No systems loaded") {
		t.Error("expected a graceful empty-systems note")
	}
	// Probes still render so the operator can gather state even with an empty registry.
	if !strings.Contains(report, "Probes — run each") {
		t.Error("probes section should render even with no systems")
	}
}

// Guard: the report must never carry obvious secret-bearing tokens. The probe
// library uses <placeholders> the operator fills locally and never pastes back.
func TestBuildOperatorReport_NoSecretsByConstruction(t *testing.T) {
	report := strings.ToLower(BuildOperatorReport(nil, defaultProbes()))
	for _, banned := range []string{"password=", "secret=", "api_key=", "bearer ", "token="} {
		if strings.Contains(report, banned) {
			t.Errorf("report unexpectedly contains a secret-bearing token: %q", banned)
		}
	}
}
