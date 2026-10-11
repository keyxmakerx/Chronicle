package vault_import

import (
	"context"
	"strings"
	"testing"
)

// analyze builds a Vault from in-memory notes. Entries named with a trailing
// .md are notes; anything else is a file.
func analyze(t *testing.T, files map[string]string) *Vault {
	t.Helper()
	var ents []zent
	for name, data := range files {
		ents = append(ents, zent{name: name, data: data})
	}
	a, err := OpenArchive(writeZip(t, ents...), "Vault.zip", DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	v, err := Analyze(context.Background(), a, func(name string) bool { return strings.HasSuffix(name, ".pdf") })
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// pageNames lists "Parent/Child" paths of every page, so a tree reads as text.
func pageNames(v *Vault) map[string]bool {
	out := map[string]bool{}
	for _, p := range v.Pages {
		name := p.Name
		for a := p.Parent; a >= 0; a = v.Pages[a].Parent {
			name = v.Pages[a].Name + "/" + name
		}
		out[name] = true
	}
	return out
}

func noteIndex(t *testing.T, v *Vault, path string) int {
	t.Helper()
	for i, n := range v.Notes {
		if n.Entry.Path == path {
			return i
		}
	}
	t.Fatalf("no note %q in %v", path, pathsOf(v))
	return -1
}

func pathsOf(v *Vault) []string {
	var out []string
	for _, n := range v.Notes {
		out = append(out, n.Entry.Path)
	}
	return out
}

func TestAnalyze_FolderTree(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
		not   []string
	}{
		{
			name: "nested folders become parent pages",
			files: map[string]string{
				"World/Regions/North/Frostfell.md": "x",
				"World/Regions/South/Sunhold.md":   "x",
				"World/Overview.md":                "x",
				"Top.md":                           "x",
			},
			want: []string{"Top", "World", "World/Overview", "World/Regions", "World/Regions/North", "World/Regions/North/Frostfell", "World/Regions/South", "World/Regions/South/Sunhold"},
		},
		{
			name: "a note named like its folder is the folder page",
			files: map[string]string{
				"Zed.md": "x", "People/People.md": "x",
				"People/Bob.md": "x",
			},
			want: []string{"Zed", "People", "People/Bob"},
		},
		{
			name: "a sibling note named like the folder is the folder page",
			files: map[string]string{
				"Zed.md": "x", "Places/Waterdeep.md": "x",
				"Places/Waterdeep/Docks.md":  "x",
				"Places/Waterdeep/Market.md": "x",
			},
			want: []string{"Zed", "Places", "Places/Waterdeep", "Places/Waterdeep/Docks", "Places/Waterdeep/Market"},
		},
		{
			name: "index.md stands for its folder",
			files: map[string]string{
				"Zed.md": "x", "Lore/index.md": "x",
				"Lore/Gods.md": "x",
			},
			want: []string{"Zed", "Lore", "Lore/Gods"},
			not:  []string{"Lore/index"},
		},
		{
			name: "Notion ids are stripped from files and folders",
			files: map[string]string{
				"Zed.md": "x", "Campaign 0123456789abcdef0123456789abcdef/Phandalin fedcba9876543210fedcba9876543210.md": "x",
			},
			want: []string{"Zed", "Campaign", "Campaign/Phandalin"},
		},
		{
			name: "unicode names survive",
			files: map[string]string{
				"Mundo/Ñandú 🦜.md": "x",
				"東京/夜.md":          "x",
			},
			want: []string{"Mundo", "Mundo/Ñandú 🦜", "東京", "東京/夜"},
		},
		{
			name: "files at the top with no folder",
			files: map[string]string{
				"A.md": "x",
				"B.md": "x",
			},
			want: []string{"A", "B"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pageNames(analyze(t, tc.files))
			for _, w := range tc.want {
				if !got[w] {
					t.Errorf("missing page %q; have %v", w, got)
				}
			}
			for _, n := range tc.not {
				if got[n] {
					t.Errorf("unexpected page %q", n)
				}
			}
			if len(got) != len(tc.want) {
				t.Errorf("got %d pages %v, want %d %v", len(got), got, len(tc.want), tc.want)
			}
		})
	}
}

func TestAnalyze_ParentsComeBeforeChildren(t *testing.T) {
	v := analyze(t, map[string]string{"a/b/c/d.md": "x", "a/z.md": "x"})
	for i, p := range v.Pages {
		if p.Parent >= i {
			t.Errorf("page %d (%s) has parent %d which is not created earlier", i, p.Name, p.Parent)
		}
	}
}

func TestAnalyze_TitleFromFrontMatter(t *testing.T) {
	v := analyze(t, map[string]string{
		"a.md": "---\nname: Lord Bob\ntags: [x]\nvisibility: public\n---\ntext",
		"b.md": "---\nname: [broken\n---\ntext",
	})
	got := pageNames(v)
	if !got["Lord Bob"] || !got["b"] {
		t.Errorf("pages = %v, want Lord Bob and b", got)
	}
	if v.Stats.BadFrontMatter != 1 || v.Stats.TagPages != 1 || v.Stats.VisibilityKeys != 1 {
		t.Errorf("stats = %+v, want 1 bad header, 1 tagged page, 1 visibility key", v.Stats)
	}
}

func TestVault_Resolve(t *testing.T) {
	v := analyze(t, map[string]string{
		"People/Bob.md":        "x",
		"People/Alice.md":      "x",
		"Places/Bob.md":        "x",
		"Deep/A/B/Quiet.md":    "x",
		"Notes/Home.md":        "x",
		"Places/Inn.md":        "x",
		"img/map.png":          "png",
		"Places/inn photo.png": "png",
		"docs/rules.pdf":       "pdf",
		"Ünï/Café.md":          "x",
	})
	src := noteIndex(t, v, "Notes/Home.md")
	inn := noteIndex(t, v, "Places/Inn.md")

	tests := []struct {
		name     string
		from     int
		ref      Ref
		wantKind TargetKind
		wantPath string
	}{
		{"bare name", src, Ref{Kind: RefWiki, Target: "Alice"}, TargetNote, "People/Alice.md"},
		{"case-insensitive", src, Ref{Kind: RefWiki, Target: "alice"}, TargetNote, "People/Alice.md"},
		{"with extension", src, Ref{Kind: RefWiki, Target: "Alice.md"}, TargetNote, "People/Alice.md"},
		{"path suffix", src, Ref{Kind: RefWiki, Target: "People/Bob"}, TargetNote, "People/Bob.md"},
		{"partial path", src, Ref{Kind: RefWiki, Target: "A/B/Quiet"}, TargetNote, "Deep/A/B/Quiet.md"},
		{"tie goes to same folder", inn, Ref{Kind: RefWiki, Target: "Bob"}, TargetNote, "Places/Bob.md"},
		{"tie from elsewhere is deterministic", src, Ref{Kind: RefWiki, Target: "Bob"}, TargetNote, "People/Bob.md"},
		{"unicode, other normal form", src, Ref{Kind: RefWiki, Target: "Café"}, TargetNote, "Ünï/Café.md"},
		{"unresolved name", src, Ref{Kind: RefWiki, Target: "Nobody"}, TargetNone, ""},
		{"heading in same note", src, Ref{Kind: RefWiki, Fragment: "Goals"}, TargetSelf, ""},
		{"picture embed by name", src, Ref{Kind: RefEmbed, Target: "map.png"}, TargetFile, "img/map.png"},
		{"picture embed same folder", inn, Ref{Kind: RefEmbed, Target: "inn photo.png"}, TargetFile, "Places/inn photo.png"},
		{"attachable file", src, Ref{Kind: RefEmbed, Target: "rules.pdf"}, TargetFile, "docs/rules.pdf"},
		{"missing picture", src, Ref{Kind: RefEmbed, Target: "gone.png"}, TargetNone, ""},
		{"markdown link relative to the note", inn, Ref{Kind: RefLink, Target: "Bob.md"}, TargetNote, "Places/Bob.md"},
		{"markdown link up a folder", inn, Ref{Kind: RefLink, Target: "../People/Alice.md"}, TargetNote, "People/Alice.md"},
		{"markdown picture", src, Ref{Kind: RefImage, Target: "../img/map.png"}, TargetFile, "img/map.png"},
		{"escaping the vault resolves to nothing", src, Ref{Kind: RefLink, Target: "../../../etc/passwd.md"}, TargetNone, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := v.Resolve(tc.from, tc.ref)
			if got.Kind != tc.wantKind {
				t.Fatalf("kind = %v, want %v", got.Kind, tc.wantKind)
			}
			switch got.Kind {
			case TargetNote:
				if p := v.Notes[got.Index].Entry.Path; p != tc.wantPath {
					t.Errorf("note = %q, want %q", p, tc.wantPath)
				}
			case TargetFile:
				if p := v.Files[got.Index].Path; p != tc.wantPath {
					t.Errorf("file = %q, want %q", p, tc.wantPath)
				}
			}
		})
	}
}

func TestAnalyze_Stats(t *testing.T) {
	v := analyze(t, map[string]string{
		"Home.md":            "[[Bob]] [[Ghost]] [[Ghost]] ![[map.png]] ![[lost.png]] ![[rules.pdf]] [[Bob#Hist|him]]",
		"Bob.md":             "back to [[Home]]",
		"map.png":            "png",
		"rules.pdf":          "pdf",
		"notes.xyz":          "?",
		".obsidian/app.json": "{}",
	})
	s := v.Stats
	if s.Pages != 2 {
		t.Errorf("pages = %d, want 2", s.Pages)
	}
	if s.LinksToPages != 3 {
		t.Errorf("links to pages = %d, want 3", s.LinksToPages)
	}
	if s.LinksPlain != 2 {
		t.Errorf("plain-text links = %d, want 2 (the Ghost links)", s.LinksPlain)
	}
	if len(s.UnresolvedNames) != 1 || s.UnresolvedNames[0] != "Ghost" {
		t.Errorf("unresolved names = %v, want [Ghost]", s.UnresolvedNames)
	}
	if s.Pictures != 1 || s.PictureRefs != 1 || s.MissingPictures != 1 {
		t.Errorf("pictures = %d/%d missing %d, want 1/1 missing 1", s.Pictures, s.PictureRefs, s.MissingPictures)
	}
	if s.Files != 1 || s.FileRefs != 1 {
		t.Errorf("files = %d/%d, want 1/1", s.Files, s.FileRefs)
	}
	if s.Ignored != 1 {
		t.Errorf("ignored = %d, want 1", s.Ignored)
	}
	skipped := false
	for _, sk := range s.SkippedFiles {
		if sk.Path == "notes.xyz" {
			skipped = true
		}
	}
	if !skipped {
		t.Errorf("notes.xyz (a type pages don't accept) is not listed as skipped: %+v", s.SkippedFiles)
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Phandalin 0123456789abcdef0123456789abcdef", "Phandalin"},
		{"  Padded  ", "Padded"},
		{"0123456789abcdef0123456789abcdef", "0123456789abcdef0123456789abcdef"},
		{"Not an id abcdef", "Not an id abcdef"},
	}
	for _, tc := range tests {
		if got := DisplayName(tc.in); got != tc.want {
			t.Errorf("DisplayName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
