package systems

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	assetSVG   = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 516"><rect width="400" height="516"/></svg>`
	assetWOFF2 = "wOF2fake-font-bytes"
	assetIndex = "parts:\n  - title: P\n    chapters: [a]\n"
)

func assetBook(index string, extra map[string]string) map[string]string {
	files := map[string]string{
		"book/book.yaml":           index + assetIndex,
		"book/chapters/a.yaml":     "title: A\npages:\n  - blocks:\n      - text: hi\n",
		"book/art/cover.svg":       assetSVG,
		"book/fonts/display.woff2": assetWOFF2,
	}
	for k, v := range extra {
		files[k] = v
	}
	return files
}

func TestLoadBook_Assets(t *testing.T) {
	tests := []struct {
		name    string
		index   string
		extra   map[string]string
		wantErr string
	}{
		{"cover and font", "look: bold\ncover: art/cover.svg\nfonts:\n  - {family: Big Shoulders Display, file: fonts/display.woff2, weight: 900, style: italic}\n", nil, ""},
		{"no assets", "", nil, ""},
		{"cover outside art", "cover: fonts/display.woff2\n", nil, "cover must be art/"},
		{"cover traversal", "cover: art/../book.yaml\n", nil, "cover must be art/"},
		{"cover upper-case", "cover: art/Cover.svg\n", nil, "cover must be art/"},
		{"cover not svg", "cover: art/cover.png\n", nil, "cover must be art/"},
		{"cover missing", "cover: art/nope.svg\n", nil, "not found"},
		{"cover too big", "cover: art/cover.svg\n", map[string]string{"book/art/cover.svg": strings.Repeat("x", maxBookAssetBytes+1)}, "larger than"},
		{"font not woff2", "fonts:\n  - {family: X, file: fonts/display.ttf}\n", nil, "fonts/<name>.woff2"},
		{"font missing", "fonts:\n  - {family: X, file: fonts/nope.woff2}\n", nil, "not found"},
		{"font family injection", "fonts:\n  - {family: \"X;}body{display:none\", file: fonts/display.woff2}\n", nil, "not allowed"},
		{"font weight", "fonts:\n  - {family: X, file: fonts/display.woff2, weight: 50}\n", nil, "weight"},
		{"font style", "fonts:\n  - {family: X, file: fonts/display.woff2, style: oblique}\n", nil, "style"},
		{"font unknown key", "fonts:\n  - {family: X, file: fonts/display.woff2, size: 3}\n", nil, "size"},
		{"nine fonts", "fonts:\n" + strings.Repeat("  - {family: X, file: fonts/display.woff2}\n", 9), nil, "more than 8"},
		{"look bold ok", "look: bold\n", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeBook(t, assetBook(tt.index, tt.extra))
			b, err := LoadBook(dir, nil)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to mention %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(tt.name, "cover and font") && (b.Cover != "art/cover.svg" || len(b.Fonts) != 1 || b.Fonts[0].Weight != 900) {
				t.Errorf("cover/fonts = %q %+v", b.Cover, b.Fonts)
			}
		})
	}
}

func TestLoadBook_SymlinkedAssetRefused(t *testing.T) {
	dir := writeBook(t, assetBook("cover: art/linked.svg\n", nil))
	secret := filepath.Join(t.TempDir(), "secret.svg")
	if err := os.WriteFile(secret, []byte(assetSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, "book", "art", "linked.svg")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := LoadBook(dir, nil); err == nil || !strings.Contains(err.Error(), "plain file") {
		t.Errorf("err = %v, want a plain-file refusal", err)
	}
}

func TestBook_WithAssetBase(t *testing.T) {
	b := &Book{Cover: "art/cover.svg", Fonts: []BookFont{{Family: "X", URL: "fonts/x.woff2", Weight: 700}}}
	got := b.WithAssetBase("/b/files")
	if got.Cover != "/b/files/cover.svg" || got.Fonts[0].URL != "/b/files/x.woff2" || got.Fonts[0].Weight != 700 {
		t.Errorf("resolved = %q %+v", got.Cover, got.Fonts)
	}
	if b.Cover != "art/cover.svg" || b.Fonts[0].URL != "fonts/x.woff2" {
		t.Error("resolving mutated the source book")
	}
}

func TestBookAsset_OnlyDeclaredFiles(t *testing.T) {
	files := assetBook("cover: art/cover.svg\nfonts:\n  - {family: X, file: fonts/display.woff2}\n", map[string]string{
		"book/art/undeclared.svg": assetSVG,
		"book/fonts/liar.woff2":   "not a font",
		"book/chapters/a.yaml":    "title: A\npages:\n  - blocks:\n      - text: SECRET-CHAPTER\n",
	})
	dir := writeBook(t, files)
	tests := []struct {
		name     string
		file     string
		wantType string
		wantErr  bool
	}{
		{"cover", "cover.svg", "image/svg+xml", false},
		{"font", "display.woff2", "font/woff2", false},
		{"undeclared svg", "undeclared.svg", "", true},
		{"undeclared font", "liar.woff2", "", true},
		{"chapter file", "a.yaml", "", true},
		{"book file", "book.yaml", "", true},
		{"traversal", "../book.yaml", "", true},
		{"encoded traversal", "..%2fbook.yaml", "", true},
		{"upper-case", "Cover.svg", "", true},
		{"empty", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, ct, err := BookAsset(dir, tt.file)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("served %q", tt.file)
				}
				return
			}
			if err != nil || ct != tt.wantType || len(data) == 0 {
				t.Errorf("got %d bytes, %q, %v", len(data), ct, err)
			}
		})
	}
}

func TestBookAsset_ContentMustMatchExtension(t *testing.T) {
	dir := writeBook(t, assetBook("cover: art/cover.svg\nfonts:\n  - {family: X, file: fonts/display.woff2}\n", map[string]string{
		"book/art/cover.svg":       "<html><script>alert(1)</script></html>",
		"book/fonts/display.woff2": "<svg onload=alert(1)>",
	}))
	for _, name := range []string{"cover.svg", "display.woff2"} {
		if _, _, err := BookAsset(dir, name); err == nil {
			t.Errorf("served mislabelled %s", name)
		}
	}
}

// TestBookFileAPI: the route serves declared files with the sandbox policy,
// carries resolved URLs in the book JSON, and is behind the campaign checks.
func TestBookFileAPI(t *testing.T) {
	files := testBookFiles()
	files["book/book.yaml"] = "look: bold\ncover: art/cover.svg\nfonts:\n  - {family: Big Shoulders Display, file: fonts/display.woff2, weight: 900}\n" + files["book/book.yaml"]
	files["book/art/cover.svg"] = assetSVG
	files["book/fonts/display.woff2"] = assetWOFF2
	files["book/art/hidden.svg"] = assetSVG
	h := newBookEditHarness(t, files)

	rec := h.do("player", "GET", "/book", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("book status %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		Look  string `json:"look"`
		Cover string `json:"cover"`
		Fonts []struct {
			Family string `json:"family"`
			URL    string `json:"url"`
			Weight int    `json:"weight"`
		} `json:"fonts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	base := "/campaigns/" + testCampaign + "/systems/customds/book/files/"
	if got.Look != "bold" || got.Cover != base+"cover.svg" || len(got.Fonts) != 1 || got.Fonts[0].URL != base+"display.woff2" || got.Fonts[0].Weight != 900 {
		t.Fatalf("book json = %+v", got)
	}

	for _, tt := range []struct {
		path, ct string
		code     int
	}{
		{"/book/files/cover.svg", "image/svg+xml", http.StatusOK},
		{"/book/files/display.woff2", "font/woff2", http.StatusOK},
		{"/book/files/hidden.svg", "", http.StatusNotFound},
		{"/book/files/book.yaml", "", http.StatusNotFound},
		{"/book/files/basics.yaml", "", http.StatusNotFound},
	} {
		rec := h.do("player", "GET", tt.path, "")
		if rec.Code != tt.code {
			t.Errorf("%s: status %d, want %d", tt.path, rec.Code, tt.code)
			continue
		}
		if tt.code != http.StatusOK {
			continue
		}
		if ct := rec.Header().Get("Content-Type"); ct != tt.ct {
			t.Errorf("%s: content type %q, want %q", tt.path, ct, tt.ct)
		}
		if csp := rec.Header().Get("Content-Security-Policy"); !strings.HasPrefix(csp, "sandbox;") || !strings.Contains(csp, "default-src 'none'") {
			t.Errorf("%s: csp %q", tt.path, csp)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: no nosniff", tt.path)
		}
	}
}
