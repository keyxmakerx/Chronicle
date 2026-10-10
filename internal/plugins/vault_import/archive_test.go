package vault_import

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// zent is one entry of a test zip. A zero method means Deflate.
type zent struct {
	name   string
	data   string
	mode   os.FileMode
	method uint16
	flags  uint16
}

// writeZip builds a zip file in a temp dir and returns its path. Names are
// written exactly as given so hostile ones reach the reader.
func writeZip(t *testing.T, ents ...zent) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range ents {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.method != 0 {
			h.Method = e.method
		}
		h.Flags = e.flags
		if e.mode != 0 {
			h.SetMode(e.mode)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatalf("create %q: %v", e.name, err)
		}
		if _, err := w.Write([]byte(e.data)); err != nil {
			t.Fatalf("write %q: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "vault.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOpenArchive_RefusesHostileArchives(t *testing.T) {
	small := DefaultLimits
	small.MaxEntries = 3
	small.MaxTotalBytes = 100

	tests := []struct {
		name string
		ents []zent
		lim  Limits
	}{
		{"parent traversal", []zent{{name: "../evil.md", data: "x"}}, DefaultLimits},
		{"nested traversal", []zent{{name: "a/../../evil.md", data: "x"}}, DefaultLimits},
		{"absolute path", []zent{{name: "/etc/passwd", data: "x"}}, DefaultLimits},
		{"backslash separator", []zent{{name: `a\..\evil.md`, data: "x"}}, DefaultLimits},
		{"drive letter", []zent{{name: "C:/evil.md", data: "x"}}, DefaultLimits},
		{"control character", []zent{{name: "a\x00b.md", data: "x"}}, DefaultLimits},
		{"empty segment", []zent{{name: "a//b.md", data: "x"}}, DefaultLimits},
		{"duplicate exact name", []zent{{name: "a.md", data: "x"}, {name: "a.md", data: "y"}}, DefaultLimits},
		{"password protected", []zent{{name: "a.md", data: "x", flags: 0x1}}, DefaultLimits},
		{"too many entries", []zent{{name: "a.md"}, {name: "b.md"}, {name: "c.md"}, {name: "d.md"}}, small},
		{"declared total over cap", []zent{{name: "a.md", data: strings.Repeat("x", 80)}, {name: "b.md", data: strings.Repeat("y", 80)}}, small},
		{"compression bomb", []zent{{name: "a.md", data: strings.Repeat("\x00", 3<<20)}}, DefaultLimits},
		{"too deep", []zent{{name: strings.Repeat("d/", 30) + "a.md", data: "x"}}, DefaultLimits},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, err := OpenArchive(writeZip(t, tc.ents...), "vault.zip", tc.lim)
			if err == nil {
				_ = a.Close()
				t.Fatalf("archive was accepted, want a refusal")
			}
			if !IsArchiveError(err) {
				t.Errorf("error %v is not an ArchiveError, so the owner would see a generic failure", err)
			}
		})
	}
}

func TestOpenArchive_NotAZip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.zip")
	if err := os.WriteFile(p, []byte("this is not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := OpenArchive(p, "x.zip", DefaultLimits)
	if err == nil || !IsArchiveError(err) {
		t.Fatalf("got %v, want an ArchiveError", err)
	}
}

func TestOpenArchive_SkipsWhatIsMerelyUnsupported(t *testing.T) {
	lim := DefaultLimits
	lim.MaxFileBytes = 10
	a, err := OpenArchive(writeZip(t,
		zent{name: "v/note.md", data: "hello"},
		zent{name: "v/link.md", data: "note.md", mode: os.ModeSymlink | 0o777},
		zent{name: "v/big.pdf", data: strings.Repeat("p", 50)},
		zent{name: "v/.obsidian/app.json", data: "{}"},
		zent{name: "__MACOSX/v/._note.md", data: "x"},
		zent{name: "v/.DS_Store", data: "x"},
	), "vault.zip", lim)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()

	if len(a.Entries) != 1 || a.Entries[0].Path != "note.md" {
		t.Fatalf("entries = %+v, want only note.md", a.Entries)
	}
	if a.Name != "v" {
		t.Errorf("vault name = %q, want the single top folder v", a.Name)
	}
	if a.Ignored != 3 {
		t.Errorf("ignored = %d, want 3 hidden files", a.Ignored)
	}
	reasons := map[string]string{}
	for _, s := range a.Skipped {
		reasons[s.Path] = s.Reason
	}
	if !strings.Contains(reasons["link.md"], "shortcut") {
		t.Errorf("symlink not reported as skipped: %v", reasons)
	}
	if !strings.Contains(reasons["big.pdf"], "larger than") {
		t.Errorf("oversized file not reported as skipped: %v", reasons)
	}
}

func TestOpenArchive_CaseOnlyDuplicatesAreKeptAndReported(t *testing.T) {
	a, err := OpenArchive(writeZip(t,
		zent{name: "People/Bob.md", data: "x"},
		zent{name: "People/bob.md", data: "y"},
		// Same word, different Unicode form: precomposed vs combining accent.
		zent{name: "Caf\u00e9.md", data: "z"},
		zent{name: "Cafe\u0301.md", data: "w"},
	), "v.zip", DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	if len(a.Entries) != 4 {
		t.Errorf("entries = %d, want all 4 kept", len(a.Entries))
	}
	if len(a.CaseClashes) != 2 {
		t.Errorf("case clashes = %d, want 2", len(a.CaseClashes))
	}
}

func TestArchive_ReadNeverTrustsTheHeader(t *testing.T) {
	a, err := OpenArchive(writeZip(t, zent{name: "n.md", data: strings.Repeat("a", 100)}), "v.zip", DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	e := a.Entries[0]
	if _, err := a.Read(e, 50); err == nil {
		t.Error("read past the ceiling succeeded")
	}
	b, err := a.Read(e, 100)
	if err != nil || len(b) != 100 {
		t.Errorf("read within the ceiling: %d bytes, %v", len(b), err)
	}
}

func TestCleanEntryName(t *testing.T) {
	tests := []struct {
		in, want string
		bad      bool
	}{
		{"a/b.md", "a/b.md", false},
		{"dir/", "dir", false},
		{"Café/Ünï.md", "Café/Ünï.md", false},
		{"../a.md", "", true},
		{"a/./b.md", "", true},
		{"/a.md", "", true},
		{`a\b.md`, "", true},
		{"", "", true},
		{"a\x7f.md", "", true},
		{"bad\xffutf8.md", "", true},
		{strings.Repeat("x", 2000), "", true},
	}
	for _, tc := range tests {
		got, err := CleanEntryName(tc.in, DefaultLimits)
		if (err != nil) != tc.bad {
			t.Errorf("CleanEntryName(%q) error = %v, want error %v", tc.in, err, tc.bad)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("CleanEntryName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
