package layouts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestAssetURL_AppendsVersionToken(t *testing.T) {
	got := AssetURL("/static/css/app.css")
	if !strings.HasPrefix(got, "/static/css/app.css?v=") {
		t.Fatalf("AssetURL must preserve the path and append ?v=; got %q", got)
	}
	if token := got[strings.Index(got, "?v=")+3:]; token == "" {
		t.Error("version token must never be empty — an empty token is an unversioned URL wearing a query string")
	}
}

// TestAssetURL_LeavesForeignURLsAlone: versioning a URL Chronicle doesn't serve
// would at best be noise and at worst break a third-party request (the Google
// Fonts / cdnjs links in base.templ).
func TestAssetURL_LeavesForeignURLsAlone(t *testing.T) {
	for _, u := range []string{
		"https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css",
		"/media/uploads/map.png",
		"data:image/svg+xml,<svg/>",
		"",
		// Already carries a query/fragment — appending a second `?` would
		// corrupt it, and appending `&v=` would guess at intent.
		"/static/js/boot.js?debug=1",
		"/static/img/sprite.svg#icon",
	} {
		if got := AssetURL(u); got != u {
			t.Errorf("AssetURL(%q) = %q; want it returned untouched", u, got)
		}
	}
}

// TestAssetURL_HashesContentPerFile is the property that makes `immutable`
// caching safe: two files with different bytes must never share a token, and
// the same bytes must always produce the same one.
func TestAssetURL_HashesContentPerFile(t *testing.T) {
	fsys := fstest.MapFS{
		"a.css": &fstest.MapFile{Data: []byte(".a{color:red}")},
		"b.css": &fstest.MapFile{Data: []byte(".b{color:blue}")},
		"c.css": &fstest.MapFile{Data: []byte(".a{color:red}")}, // same bytes as a.css
	}
	withResolvers(t, []assetResolver{{prefix: "/static/probe/", fsys: fsys}})

	a := AssetURL("/static/probe/a.css")
	b := AssetURL("/static/probe/b.css")
	c := AssetURL("/static/probe/c.css")

	if tok(a) == tok(b) {
		t.Errorf("different content must produce different tokens; both = %q", tok(a))
	}
	if tok(a) != tok(c) {
		t.Errorf("identical content must produce identical tokens; %q vs %q", tok(a), tok(c))
	}
	if tok(a) == buildToken() {
		t.Error("a resolvable asset must be hashed per-file, not fall back to the per-build token")
	}
}

// TestAssetURL_UnresolvableFallsBackToBuildToken: the safety net. An asset we
// can't hash (embed FS nobody registered, path typo) must still get a token
// that changes on deploy — degrading to "coarse but correct", never to
// "unversioned and silently stale".
func TestAssetURL_UnresolvableFallsBackToBuildToken(t *testing.T) {
	withResolvers(t, []assetResolver{{prefix: "/static/probe/", fsys: fstest.MapFS{}}})
	if got := tok(AssetURL("/static/probe/missing.js")); got != buildToken() {
		t.Errorf("unresolvable asset token = %q; want the build token %q", got, buildToken())
	}
}

// TestAssetURL_NoPathEscape: a traversal in a template path must not be able to
// hash a file outside the asset root.
func TestAssetURL_NoPathEscape(t *testing.T) {
	fsys := fstest.MapFS{"a.css": &fstest.MapFile{Data: []byte("x")}}
	withResolvers(t, []assetResolver{{prefix: "/static/probe/", fsys: fsys}})
	if got := tok(AssetURL("/static/probe/../../etc/passwd")); got != buildToken() {
		t.Errorf("traversal must not resolve; token = %q, want the build fallback", got)
	}
}

func TestRegisterAssetFS_RejectsNonStaticPrefixes(t *testing.T) {
	before := len(snapshotResolvers())
	RegisterAssetFS("/media/", fstest.MapFS{})
	RegisterAssetFS("/static/ok/", nil)
	if after := len(snapshotResolvers()); after != before {
		t.Errorf("invalid registrations must be dropped; resolver count %d → %d", before, after)
	}
}

// TestTemplatesUseAssetURL is the C-ASSET-VERSIONING contract test the dispatch
// asks for: no .templ file may emit a bare /static/ URL. A single missed
// conversion is exactly how the stale-asset class comes back.
func TestTemplatesUseAssetURL(t *testing.T) {
	root := repoRoot(t)
	var offenders []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == ".git" || name == "node_modules" || name == "static" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(p) != ".templ" {
			return nil
		}
		body, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(root, p)
		for i, line := range strings.Split(string(body), "\n") {
			// A bare `"/static/...` string literal in an attribute is the
			// unversioned form. The AssetURL call sites read
			// `AssetURL("/static/...")`, so exclude those explicitly.
			for _, attr := range []string{`src="` + StaticURLPrefix, `href="` + StaticURLPrefix} {
				if strings.Contains(line, attr) {
					offenders = append(offenders, rel+":"+itoa(i+1)+": "+strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(offenders) > 0 {
		t.Errorf("these templates emit unversioned static URLs — route them through layouts.AssetURL(...) so a deploy busts client caches:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

// --- helpers -------------------------------------------------------------

func tok(url string) string {
	i := strings.Index(url, "?v=")
	if i < 0 {
		return ""
	}
	return url[i+3:]
}

func snapshotResolvers() []assetResolver {
	assetMu.RLock()
	defer assetMu.RUnlock()
	return assetResolvers
}

// withResolvers swaps the resolver list (and clears the memo cache) for one
// test, restoring both afterwards.
func withResolvers(t *testing.T, rs []assetResolver) {
	t.Helper()
	assetMu.Lock()
	prev := assetResolvers
	assetResolvers = rs
	assetMu.Unlock()
	clearDigests()
	t.Cleanup(func() {
		assetMu.Lock()
		assetResolvers = prev
		assetMu.Unlock()
		clearDigests()
	})
}

func clearDigests() {
	assetDigests.Range(func(k, _ any) bool {
		assetDigests.Delete(k)
		return true
	})
}

// repoRoot walks up from the package dir to the directory holding go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root (no go.mod above the test's working directory)")
		}
		dir = parent
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
