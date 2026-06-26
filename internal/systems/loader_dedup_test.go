package systems

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestManifestVer is writeTestManifest with an explicit version, for the
// duplicate-resolution tests (WS-6). status "coming_soon" keeps discovery from
// attempting instantiation (no data dir in these temp trees) — registration into
// the modules map, which is what these tests assert on, happens regardless.
func writeTestManifestVer(t *testing.T, dir, id, name, version string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("creating dir %s: %v", dir, err)
	}
	data := `{
		"id": "` + id + `",
		"name": "` + name + `",
		"version": "` + version + `",
		"api_version": "1",
		"status": "coming_soon",
		"categories": [{"slug": "items", "name": "Items"}]
	}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(data), 0644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
}

// A newer copy in a LATER-sorted directory must win over an older one scanned
// first (the plain last-wins bug would have kept whichever sorted last).
func TestSystemLoader_DuplicateID_NewestWins(t *testing.T) {
	base := t.TempDir()
	writeTestManifestVer(t, filepath.Join(base, "alpha"), "dup", "Dup", "0.1.0")
	writeTestManifestVer(t, filepath.Join(base, "zeta"), "dup", "Dup", "0.2.0")

	l := NewSystemLoader(base)
	if err := l.DiscoverAll(); err != nil {
		t.Fatalf("DiscoverAll: %v", err)
	}
	if l.Count() != 1 {
		t.Fatalf("Count = %d, want 1 (deduped)", l.Count())
	}
	if got := l.Get("dup"); got == nil || got.Version != "0.2.0" {
		t.Fatalf("kept version = %v, want 0.2.0", got)
	}
	if dir := l.Dir("dup"); !strings.HasSuffix(dir, "zeta") {
		t.Fatalf("kept dir = %q, want .../zeta", dir)
	}
}

// A stale/older duplicate in a later directory must NOT shadow the current,
// newer copy scanned first — the actual "<slug>-1" leftover hazard.
func TestSystemLoader_DuplicateID_StaleDoesNotShadow(t *testing.T) {
	base := t.TempDir()
	writeTestManifestVer(t, filepath.Join(base, "alpha"), "dup", "Dup", "0.2.0")
	writeTestManifestVer(t, filepath.Join(base, "zeta-1"), "dup", "Dup", "0.1.0")

	l := NewSystemLoader(base)
	if err := l.DiscoverAll(); err != nil {
		t.Fatalf("DiscoverAll: %v", err)
	}
	if l.Count() != 1 {
		t.Fatalf("Count = %d, want 1", l.Count())
	}
	if got := l.Get("dup"); got == nil || got.Version != "0.2.0" {
		t.Fatalf("kept version = %v, want 0.2.0 (stale must not win)", got)
	}
	if dir := l.Dir("dup"); !strings.HasSuffix(dir, "alpha") {
		t.Fatalf("kept dir = %q, want .../alpha", dir)
	}
}

func TestPreferCandidate(t *testing.T) {
	l := NewSystemLoader("")
	bundled := &loadedSystem{
		manifest: &SystemManifest{ID: "x", Version: "1.0.0"},
		dir:      "b", source: "bundled",
	}
	cand := func(v string) *SystemManifest { return &SystemManifest{ID: "x", Version: v} }

	cases := []struct {
		name     string
		existing *loadedSystem
		version  string
		source   string
		want     bool
	}{
		{"nil existing accepts", nil, "0.0.1", "bundled", true},
		{"newer wins", bundled, "1.1.0", "bundled", true},
		{"older never downgrades", bundled, "0.9.0", "package", false},
		{"equal: package overlays bundled", bundled, "1.0.0", "package", true},
		{"equal: bundled keeps first", bundled, "1.0.0", "bundled", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := l.preferCandidate(c.existing, cand(c.version), c.source); got != c.want {
				t.Errorf("preferCandidate = %v, want %v", got, c.want)
			}
		})
	}
}
