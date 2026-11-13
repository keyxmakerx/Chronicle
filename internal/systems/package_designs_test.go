package systems

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeKinds knows one kind, "room-theme", whose data must be {"part":"oak"}.
type fakeKinds struct{}

func (fakeKinds) ValidateDesign(_ context.Context, kind string, raw json.RawMessage) (json.RawMessage, error) {
	if kind != "room-theme" {
		return nil, errors.New("not a kind of design this server knows")
	}
	var v struct {
		Part string `json:"part"`
	}
	if json.Unmarshal(raw, &v) != nil || v.Part != "oak" {
		return nil, errors.New("unknown part")
	}
	return json.RawMessage(`{"part":"oak"}`), nil
}

func writePkg(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestManifestDesignsStructure(t *testing.T) {
	many := make([]DesignDef, MaxPackageDesigns+3)
	for i := range many {
		many[i] = DesignDef{Kind: "room-theme", Name: "n", File: "designs/a.json"}
	}
	cases := []struct {
		name      string
		in        []DesignDef
		wantKept  int
		wantWarns int
	}{
		{"good", []DesignDef{{Kind: "room-theme", Name: "Ashgate", Note: "n", Maker: "M", File: "designs/a.json"}}, 1, 0},
		{"no kind", []DesignDef{{Name: "x", File: "a.json"}}, 0, 1},
		{"no name", []DesignDef{{Kind: "k", File: "a.json"}}, 0, 1},
		{"name too long", []DesignDef{{Kind: "k", Name: strings.Repeat("a", 61), File: "a.json"}}, 0, 1},
		{"name at cap", []DesignDef{{Kind: "k", Name: strings.Repeat("a", 60), File: "a.json"}}, 1, 0},
		{"note too long", []DesignDef{{Kind: "k", Name: "x", Note: strings.Repeat("a", 141), File: "a.json"}}, 0, 1},
		{"dotdot", []DesignDef{{Kind: "k", Name: "x", File: "../a.json"}}, 0, 1},
		{"nested dotdot", []DesignDef{{Kind: "k", Name: "x", File: "designs/../../a.json"}}, 0, 1},
		{"absolute", []DesignDef{{Kind: "k", Name: "x", File: "/etc/a.json"}}, 0, 1},
		{"backslash", []DesignDef{{Kind: "k", Name: "x", File: `designs\a.json`}}, 0, 1},
		{"not json", []DesignDef{{Kind: "k", Name: "x", File: "designs/a.txt"}}, 0, 1},
		{"one bad one good", []DesignDef{{Kind: "k", Name: "x", File: "../a.json"}, {Kind: "k", Name: "y", File: "a.json"}}, 1, 1},
		{"too many", many, MaxPackageDesigns, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &SystemManifest{ID: "s", Name: "S", APIVersion: "1", Designs: tc.in}
			if err := ValidateManifest(m); err != nil {
				t.Fatalf("a bad design must not fail the package: %v", err)
			}
			if len(m.Designs) != tc.wantKept || len(m.DesignWarnings) != tc.wantWarns {
				t.Fatalf("kept %d warnings %v, want %d and %d", len(m.Designs), m.DesignWarnings, tc.wantKept, tc.wantWarns)
			}
			// Revalidating must not duplicate the warnings or lose entries.
			_ = ValidateManifest(m)
			if len(m.Designs) != tc.wantKept || len(m.DesignWarnings) != tc.wantWarns {
				t.Fatalf("second pass changed things: %d %v", len(m.Designs), m.DesignWarnings)
			}
			if got := m.BuildValidationReport().Warnings; tc.wantWarns > 0 && !containsStr(got, m.DesignWarnings[0]) {
				t.Fatalf("report lacks the design warning: %v", got)
			}
		})
	}
}

func TestLoadPackageDesigns(t *testing.T) {
	big := `{"part":"oak","pad":"` + strings.Repeat("x", MaxPackageDesignBytes) + `"}`
	dir := writePkg(t, map[string]string{
		"designs/good.json":    `{"part":"oak"}`,
		"designs/bad.json":     `{"part":"ivory"}`,
		"designs/broken.json":  `{nope`,
		"designs/big.json":     big,
		"designs/sub/dir.json": `{"part":"oak"}`,
	})
	// A symlink out of the package must not be followed.
	outside := writePkg(t, map[string]string{"secret.json": `{"part":"oak"}`})
	if err := os.Symlink(filepath.Join(outside, "secret.json"), filepath.Join(dir, "designs", "link.json")); err != nil {
		t.Skip("no symlinks here")
	}
	m := &SystemManifest{ID: "s", Name: "S", APIVersion: "1", Designs: []DesignDef{
		{Kind: "room-theme", Name: "Good", File: "designs/good.json"},
		{Kind: "room-theme", Name: "Nested", File: "designs/sub/dir.json"},
		{Kind: "mystery", Name: "Unknown kind", File: "designs/good.json"},
		{Kind: "room-theme", Name: "Bad data", File: "designs/bad.json"},
		{Kind: "room-theme", Name: "Broken", File: "designs/broken.json"},
		{Kind: "room-theme", Name: "Oversize", File: "designs/big.json"},
		{Kind: "room-theme", Name: "Missing", File: "designs/missing.json"},
		{Kind: "room-theme", Name: "Linked out", File: "designs/link.json"},
	}}
	got, skipped := LoadPackageDesigns(context.Background(), m, dir, fakeKinds{})
	if len(got) != 2 || got[0].Name != "Good" || got[1].Name != "Nested" {
		t.Fatalf("loaded %v", got)
	}
	if len(skipped) != 6 {
		t.Fatalf("skipped %d: %v", len(skipped), skipped)
	}
	for _, want := range []string{"Unknown kind", "Bad data", "Broken", "Oversize", "Missing", "Linked out"} {
		found := false
		for _, s := range skipped {
			found = found || strings.Contains(s, want)
		}
		if !found {
			t.Errorf("no diagnostic for %q in %v", want, skipped)
		}
	}
}

func TestLoadPackageDesignsWithoutKinds(t *testing.T) {
	dir := writePkg(t, map[string]string{"a.json": `{"part":"oak"}`})
	m := &SystemManifest{Designs: []DesignDef{{Kind: "room-theme", Name: "A", File: "a.json"}}}
	got, skipped := LoadPackageDesigns(context.Background(), m, dir, nil)
	if len(got) != 0 || len(skipped) != 1 {
		t.Fatalf("nothing may load unchecked: %v %v", got, skipped)
	}
}

// A suggestion file's entry and data must be accepted as they stand when a
// maintainer pastes them: the entry into the manifest, the data into its file.
func TestSuggestionEntryRoundTripsThroughTheManifest(t *testing.T) {
	const export = `{"format":"chronicle-design-suggestion:v1","system":"s","designs":[{"kind":"room-theme","name":"Moonlit Ward","note":"n","maker":"Alice","file":"designs/moonlit-ward.json"}],"data":{"designs/moonlit-ward.json":{"part":"oak"}}}`
	var sg struct {
		Designs []DesignDef                `json:"designs"`
		Data    map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(export), &sg); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for f, d := range sg.Data {
		files[f] = string(d)
	}
	m := &SystemManifest{ID: "s", Name: "S", APIVersion: "1", Designs: sg.Designs}
	if err := ValidateManifest(m); err != nil || len(m.DesignWarnings) != 0 {
		t.Fatalf("manifest: %v %v", err, m.DesignWarnings)
	}
	got, skipped := LoadPackageDesigns(context.Background(), m, writePkg(t, files), fakeKinds{})
	if len(got) != 1 || len(skipped) != 0 || got[0].Maker != "Alice" {
		t.Fatalf("%v %v", got, skipped)
	}
}
