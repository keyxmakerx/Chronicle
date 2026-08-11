package systems

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// The asset diagnostics exist because two storage mechanisms that look nothing
// alike from a shell both serve Chronicle's front-end. These tests lead with the
// paths that MISLEAD — a refused traversal that must not read as "not found",
// an unwired provider that must not read as "no embedded assets", a served token
// that no longer matches the bytes — because those are the answers an operator
// acts on, and a renderer that only worked on the happy path would stay green
// forever while telling someone the wrong thing.

// ── clamping ───────────────────────────────────────────────────────────────

// TestClampStaticRelRefusals pins that an escape attempt is REFUSED with its
// reason, not silently resolved. The absolute-path case is the subtle one:
// filepath.Join("<root>", "/etc/passwd") is "<root>/etc/passwd", which sits
// happily inside the root — so a clamp built only on prefix-checking would
// accept it and then report "not found", sending a reader after a missing file
// instead of a rejected path.
func TestClampStaticRelRefusals(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name    string
		rel     string
		wantSub string
	}{
		{"parent traversal", "../secret.txt", "escapes the static root"},
		{"deep traversal", "css/../../../etc/passwd", "escapes the static root"},
		{"absolute unix path", "/etc/passwd", "absolute paths are refused"},
		{"absolute inside root", filepath.Join(root, "a.css"), "absolute paths are refused"},
		{"empty", "  ", "the path is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			full, refusal := clampStaticRel(root, tc.rel)
			if refusal == "" {
				t.Fatalf("clampStaticRel(%q) accepted it and returned %q; it must refuse", tc.rel, full)
			}
			if full != "" {
				t.Errorf("a refused path must yield no usable path, got %q", full)
			}
			if !strings.Contains(refusal, tc.wantSub) {
				t.Errorf("refusal = %q, want it to contain %q", refusal, tc.wantSub)
			}
		})
	}
}

// TestClampStaticRelAccepts confirms the clamp is not simply refusing
// everything — a guard that never lets anything through passes the test above
// while making the diagnostic useless.
func TestClampStaticRelAccepts(t *testing.T) {
	root := t.TempDir()
	full, refusal := clampStaticRel(root, "css/app.css")
	if refusal != "" {
		t.Fatalf("clampStaticRel refused a plain relative path: %s", refusal)
	}
	if want := filepath.Join(root, "css", "app.css"); full != want {
		t.Errorf("full = %q, want %q", full, want)
	}
}

// TestClampEmbeddedRelRefusals pins the same guarantee on the fs.FS side, where
// the rules are fs.ValidPath's rather than the filesystem's.
func TestClampEmbeddedRelRefusals(t *testing.T) {
	cases := []struct{ rel, wantSub string }{
		{"../x.js", "escapes the plugin's embedded root"},
		{"js/../../x.js", "escapes the plugin's embedded root"},
		{"/js/x.js", "absolute paths are refused"},
		{"", "the path is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.rel, func(t *testing.T) {
			got, refusal := clampEmbeddedRel(tc.rel)
			if refusal == "" {
				t.Fatalf("clampEmbeddedRel(%q) accepted it and returned %q", tc.rel, got)
			}
			if !strings.Contains(refusal, tc.wantSub) {
				t.Errorf("refusal = %q, want it to contain %q", refusal, tc.wantSub)
			}
		})
	}
	if got, refusal := clampEmbeddedRel("js/gm_panel.js"); refusal != "" || got != "js/gm_panel.js" {
		t.Errorf("a valid embedded path was refused: got %q, refusal %q", got, refusal)
	}
}

// TestAssetContainsRefusesTraversalInOutput pins that the REFUSAL REACHES THE
// OPERATOR. The clamp being correct is only half of it — the brief's
// requirement is that the output says a path was rejected, so a reader never
// mistakes "I would not look there" for "it is not there".
func TestAssetContainsRefusesTraversalInOutput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "keep.css"), []byte("body{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	sr := staticRoot{CWD: filepath.Dir(root), Path: root, Exists: true}

	for _, arg := range []string{"../../etc/passwd:root", "/etc/passwd:root"} {
		out := renderHostAssetContainsFrom(sr, arg)
		if !strings.Contains(out, "Refused") {
			t.Errorf("arg %q: output does not say it refused the path:\n%s", arg, out)
		}
		if !strings.Contains(out, "Nothing was read") {
			t.Errorf("arg %q: output does not state that nothing was read:\n%s", arg, out)
		}
		if strings.Contains(out, "FOUND") || strings.Contains(out, "not found") {
			t.Errorf("arg %q: a refused path must not produce marker results:\n%s", arg, out)
		}
	}
}

// ── markers ────────────────────────────────────────────────────────────────

// TestAssetContainsMarkersAgainstRealFile runs the marker check against a file
// that actually ships in this repo, using a marker that is an architectural
// contract (CLAUDE.md: widgets "mount to a DOM element via data-widget
// attributes", auto-mounted by boot.js) rather than an incidental token. A
// synthetic temp file would exercise strings.Contains; this exercises the real
// resolution path from the static root down to bytes on disk.
func TestAssetContainsMarkersAgainstRealFile(t *testing.T) {
	repoStatic := filepath.Join("..", "..", staticDirName)
	abs, err := filepath.Abs(repoStatic)
	if err != nil {
		t.Fatalf("resolving the repo static root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(abs, "js", "boot.js")); err != nil {
		t.Skipf("repo static root not present at %s (%v)", abs, err)
	}
	sr := staticRoot{CWD: filepath.Dir(abs), Path: abs, Exists: true}

	out := renderHostAssetContainsFrom(sr, "js/boot.js:data-widget,__marker_that_is_definitely_absent__")
	if !strings.Contains(out, "✓ `data-widget` — FOUND") {
		t.Errorf("present marker was not reported as found:\n%s", out)
	}
	if !strings.Contains(out, "✗ `__marker_that_is_definitely_absent__` — not found") {
		t.Errorf("absent marker was not reported as not-found:\n%s", out)
	}
	// The fingerprint has to be there too: a marker answer without the hash and
	// mtime cannot be compared against another deploy.
	if !strings.Contains(out, "bytes ·") {
		t.Errorf("output is missing the size/sha256/mtime fingerprint line:\n%s", out)
	}
}

// TestAssetContainsAcceptsBothPathForms keeps the tool forgiving about the two
// ways an operator will type the path — the URL and the repo-relative path —
// and pins that it SAYS it stripped a prefix rather than doing it invisibly.
func TestAssetContainsAcceptsBothPathForms(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "css"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "css", "calendar-block.css"), []byte(".moonpick{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	sr := staticRoot{CWD: filepath.Dir(root), Path: root, Exists: true}

	for _, arg := range []string{
		"css/calendar-block.css:moonpick",
		"static/css/calendar-block.css:moonpick",
		"/static/css/calendar-block.css:moonpick",
	} {
		out := renderHostAssetContainsFrom(sr, arg)
		if !strings.Contains(out, "✓ `moonpick` — FOUND") {
			t.Errorf("arg %q did not find the marker:\n%s", arg, out)
		}
	}
	if out := renderHostAssetContainsFrom(sr, "/static/css/calendar-block.css:moonpick"); !strings.Contains(out, "Stripped the leading") {
		t.Errorf("stripping a prefix must be stated in the output:\n%s", out)
	}
}

// TestAssetContainsMissingFilePointsAtEmbedded is the incident, encoded. Someone
// looking for a plugin's asset under the static root will not find it there,
// ever, and the miss must hand them the next step instead of confirming their
// wrong hypothesis.
func TestAssetContainsMissingFilePointsAtEmbedded(t *testing.T) {
	root := t.TempDir()
	sr := staticRoot{CWD: filepath.Dir(root), Path: root, Exists: true}
	out := renderHostAssetContainsFrom(sr, "css/gm_panel.css:moon")
	if !strings.Contains(out, "not found") {
		t.Fatalf("expected a not-found report:\n%s", out)
	}
	if !strings.Contains(out, "host.embedded") {
		t.Errorf("a miss under the static root must point at host.embedded:\n%s", out)
	}
}

// ── unwired provider ───────────────────────────────────────────────────────

// TestEmbeddedUnwiredProviderDegrades pins that a nil provider produces a
// MESSAGE, not a panic — and specifically that it does not read as "there are
// no embedded assets". Those are opposite conclusions for someone hunting code
// that looks missing, which is the only reason anyone runs this diagnostic.
func TestEmbeddedUnwiredProviderDegrades(t *testing.T) {
	out := renderHostEmbeddedFrom(nil, "", nil, "tok")
	if !strings.Contains(out, "Provider not wired") {
		t.Errorf("nil provider must say so:\n%s", out)
	}
	if !strings.Contains(out, "says nothing about whether embedded assets exist") {
		t.Errorf("nil provider must NOT read as 'no embedded assets':\n%s", out)
	}

	out = renderHostEmbeddedContainsFrom(nil, "calendar:js/gm_panel.js:moon")
	if !strings.Contains(out, "Provider not wired") {
		t.Errorf("nil provider must say so for embedded-contains:\n%s", out)
	}
}

// TestEmbeddedWiredButEmptyIsADifferentAnswer separates "nobody told me" from
// "I looked and there is nothing" — the distinction the previous test protects
// only matters if the empty case is actually worded as an answer.
func TestEmbeddedWiredButEmptyIsADifferentAnswer(t *testing.T) {
	out := renderHostEmbeddedFrom(func() []EmbeddedAssetSet { return nil }, "", nil, "tok")
	if strings.Contains(out, "Provider not wired") {
		t.Errorf("a wired provider must not report itself as unwired:\n%s", out)
	}
	if !strings.Contains(out, "provider is wired") {
		t.Errorf("the empty case must state that it IS an answer:\n%s", out)
	}
}

// ── embedded listing + content ─────────────────────────────────────────────

// fakeEmbedded mirrors the real shape: an FS already rooted at the plugin's
// static/ dir, so paths are "js/x.js" and not "static/js/x.js" (measured
// against echo.MustSubFS(<embed.FS>, "static"), which is what the app passes).
func fakeEmbedded() []EmbeddedAssetSet {
	return []EmbeddedAssetSet{
		{Slug: "calendar", URLPrefix: "/static/plugins/calendar/", FS: fstest.MapFS{
			"css/gm_panel.css": &fstest.MapFile{Data: []byte(".moonpick{color:red}")},
			"js/gm_panel.js":   &fstest.MapFile{Data: []byte("function moonPhase(){}")},
		}},
		{Slug: "entities", URLPrefix: "/static/plugins/entities/", FS: fstest.MapFS{
			"js/characters.js": &fstest.MapFile{Data: []byte("// characters")},
		}},
	}
}

// TestEmbeddedListsFilesAndSaysTheyAreNotOnDisk pins the sentence the whole
// diagnostic exists for. Without it the listing is just an inventory; with it,
// the reader learns why their grep came back empty.
func TestEmbeddedListsFilesAndSaysTheyAreNotOnDisk(t *testing.T) {
	out := renderHostEmbeddedFrom(fakeEmbedded, "", nil, "tok")

	if !strings.Contains(out, "not on disk") {
		t.Errorf("the 'not on disk' statement is missing:\n%s", out)
	}
	if !strings.Contains(out, "grep") {
		t.Errorf("the output must warn that grepping the filesystem finds nothing:\n%s", out)
	}
	for _, want := range []string{"`calendar`", "`entities`", "css/gm_panel.css", "js/characters.js", "/static/plugins/calendar/"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// embed.FS records no mtimes (measured: every entry reports the zero time),
	// so a printed mtime would be fabricated. Pin its absence and its reason.
	if strings.Contains(out, "0001-01-01") {
		t.Errorf("embedded listing printed a zero mtime as if it were data:\n%s", out)
	}
	if !strings.Contains(out, "No mtime column") {
		t.Errorf("the missing mtime column must be explained, not just omitted:\n%s", out)
	}
}

// TestEmbeddedFallbackAdviceIsNotTheOnDiskAdvice guards a wording bug found by
// running the real thing: the shared token verdict used to advise "check the
// working directory", which is meaningless for bytes compiled into the binary.
// Embedded assets have no working directory; a fallback there means the plugin
// FS was mounted for serving but never registered for hashing. Confidently
// wrong advice is the failure mode this whole workstream exists to prevent.
func TestEmbeddedFallbackAdviceIsNotTheOnDiskAdvice(t *testing.T) {
	stub := func(p string) string { return p + "?v=buildtoken" }
	out := renderHostEmbeddedFrom(fakeEmbedded, "", stub, "buildtoken")
	if !strings.Contains(out, "BUILD-TOKEN FALLBACK") {
		t.Fatalf("expected the fallback verdict:\n%s", out)
	}
	if strings.Contains(out, "check the working directory") {
		t.Errorf("embedded output must not advise checking a working directory:\n%s", out)
	}
	if !strings.Contains(out, "never registered for content hashing") {
		t.Errorf("embedded output must name its OWN cause for the fallback:\n%s", out)
	}
}

// TestEmbeddedSlugFilter checks the argument narrows to one plugin and that an
// unknown slug lists the real ones rather than reporting a bare miss.
func TestEmbeddedSlugFilter(t *testing.T) {
	out := renderHostEmbeddedFrom(fakeEmbedded, "calendar", nil, "tok")
	if !strings.Contains(out, "css/gm_panel.css") {
		t.Errorf("filter dropped the plugin's own files:\n%s", out)
	}
	if strings.Contains(out, "js/characters.js") {
		t.Errorf("filter leaked another plugin's files:\n%s", out)
	}

	out = renderHostEmbeddedFrom(fakeEmbedded, "nope", nil, "tok")
	if !strings.Contains(out, "`calendar`") || !strings.Contains(out, "`entities`") {
		t.Errorf("an unknown slug must list the available ones:\n%s", out)
	}
}

// TestEmbeddedContainsMarkers is the present/absent pair against embedded bytes
// — the check that has no on-disk equivalent, because there is no file to grep.
func TestEmbeddedContainsMarkers(t *testing.T) {
	out := renderHostEmbeddedContainsFrom(fakeEmbedded, "calendar:js/gm_panel.js:moonPhase,__absent__")
	if !strings.Contains(out, "✓ `moonPhase` — FOUND") {
		t.Errorf("present marker not reported:\n%s", out)
	}
	if !strings.Contains(out, "✗ `__absent__` — not found") {
		t.Errorf("absent marker not reported:\n%s", out)
	}
	if !strings.Contains(out, "EMBEDDED filesystem (not from disk)") {
		t.Errorf("the output must say where the bytes came from:\n%s", out)
	}
}

// TestEmbeddedContainsRefusesTraversal pins the clamp at the diagnostic's own
// surface, not only in the helper.
func TestEmbeddedContainsRefusesTraversal(t *testing.T) {
	for _, arg := range []string{"calendar:../../../etc/passwd:root", "calendar:/etc/passwd:root"} {
		out := renderHostEmbeddedContainsFrom(fakeEmbedded, arg)
		if !strings.Contains(out, "Refused") {
			t.Errorf("arg %q was not refused:\n%s", arg, out)
		}
		if strings.Contains(out, "FOUND") {
			t.Errorf("arg %q produced marker results despite refusal:\n%s", arg, out)
		}
	}
}

// TestEmbeddedContainsUnknownPathPointsAtTheListing — a wrong path is the most
// likely operator error here (the paths have no `static/` prefix, unlike every
// other path in the system), so the miss must name the diagnostic that lists them.
func TestEmbeddedContainsUnknownPathPointsAtTheListing(t *testing.T) {
	out := renderHostEmbeddedContainsFrom(fakeEmbedded, "calendar:static/js/gm_panel.js:moonPhase")
	if !strings.Contains(out, "not found") {
		t.Fatalf("expected a not-found report:\n%s", out)
	}
	if !strings.Contains(out, "host.embedded calendar") {
		t.Errorf("a miss must point at the listing diagnostic:\n%s", out)
	}
}

// ── static root resolution + ?v= verdicts ──────────────────────────────────

// TestAssetsUnusableStaticRootIsLoud is the failure a hardcoded /app/static
// could never surface: the process looking somewhere that is not there. It must
// print the working directory it resolved against, because that is the fact
// that identifies the mistake.
func TestAssetsUnusableStaticRootIsLoud(t *testing.T) {
	sr := staticRoot{CWD: "/somewhere", Path: "/somewhere/static", StatErr: "no such file or directory"}
	out := renderHostAssetsFrom(sr, "", nil, "tok")
	if !strings.Contains(out, "THE STATIC ROOT IS NOT USABLE") {
		t.Errorf("an absent static root must be loud:\n%s", out)
	}
	if !strings.Contains(out, "/somewhere") {
		t.Errorf("the resolved working directory must be printed:\n%s", out)
	}
}

// TestTokenVerdicts pins the three ⚠️ conditions apart. They look identical in
// a browser (every one still produces a cache-busted URL) and mean completely
// different things, which is exactly why the column exists.
func TestTokenVerdicts(t *testing.T) {
	const live = "abcdef0123456789" // sha256[:16]; AssetURL emits the first 10
	cases := []struct{ name, token, want string }{
		{"content hash agrees", "abcdef0123", "✓ matches bytes on disk"},
		{"build-token fallback", "buildtoken", "BUILD-TOKEN FALLBACK"},
		{"stale memoised digest", "9999999999", "STALE"},
		{"not versioned at all", "", "NOT VERSIONED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tokenVerdict(tc.token, live, "buildtoken"); !strings.Contains(got, tc.want) {
				t.Errorf("tokenVerdict(%q) = %q, want it to contain %q", tc.token, got, tc.want)
			}
		})
	}
}

// TestAssetsReportsServedTokenPerFile drives the whole renderer over a real
// temp tree with an injected AssetURL, covering the default css/js selection,
// the "rest by count" summary, and the divergence flagging in one pass.
func TestAssetsReportsServedTokenPerFile(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("css/app.css", "body{}")
	write("js/boot.js", "console.log(1)")
	write("img/logo.svg", "<svg/>")
	write("fonts/a.woff2", "x")

	// A stub AssetURL that returns the build token for everything: the
	// "app cannot resolve these files" condition, which is what a wrong working
	// directory actually looks like.
	stub := func(p string) string { return p + "?v=buildtoken" }

	out := renderHostAssetsFrom(staticRoot{CWD: root, Path: root, Exists: true}, "", stub, "buildtoken")
	for _, want := range []string{"css/app.css", "js/boot.js", "?v=buildtoken", "BUILD-TOKEN FALLBACK"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "img/logo.svg") {
		t.Errorf("no-arg listing must be css/js only:\n%s", out)
	}
	if !strings.Contains(out, "2 other file(s) not listed") {
		t.Errorf("the unlisted files must be summarised by count:\n%s", out)
	}
	if !strings.Contains(out, "2 of 2 file(s) do NOT agree") {
		t.Errorf("divergences must be counted in the trailing note:\n%s", out)
	}

	// A substring argument reaches past the css/js default.
	out = renderHostAssetsFrom(staticRoot{CWD: root, Path: root, Exists: true}, "img/", stub, "buildtoken")
	if !strings.Contains(out, "img/logo.svg") {
		t.Errorf("the substring filter did not reach a non-css/js file:\n%s", out)
	}
	if strings.Contains(out, "css/app.css") {
		t.Errorf("the substring filter leaked non-matching files:\n%s", out)
	}
}

// ── catalog registration ───────────────────────────────────────────────────

// TestAssetDiagnosticsAreRegistered pins that all four are reachable by name
// and that host.embedded's description carries the not-on-disk warning — the
// catalog listing is where an assistant chooses a diagnostic, so the sentence
// has to survive there, not only in the result.
func TestAssetDiagnosticsAreRegistered(t *testing.T) {
	cat := diagnosticCatalog()
	byName := map[string]Diagnostic{}
	for _, d := range cat {
		byName[d.Name] = d
	}
	for _, n := range []string{"host.assets", "host.asset-contains", "host.embedded", "host.embedded-contains"} {
		d, ok := byName[n]
		if !ok {
			t.Fatalf("%s is not in the catalog", n)
		}
		if d.Run == nil {
			t.Errorf("%s has no Run func", n)
		}
		if d.ArgHint == "" {
			t.Errorf("%s should advertise its argument", n)
		}
	}
	desc := byName["host.embedded"].Desc
	if !strings.Contains(desc, "NOT ON DISK") && !strings.Contains(desc, "NOT on disk") {
		t.Errorf("host.embedded's Desc must say plainly that the files are not on disk; got: %s", desc)
	}
	if !strings.Contains(desc, "grep") {
		t.Errorf("host.embedded's Desc must say grepping the filesystem finds nothing; got: %s", desc)
	}

	// Ordering: the asset diagnostics belong with host.build/host.runtime at the
	// top, ahead of every system.*/packages.* check, for the same reason —
	// "what is being served" is unanswerable until "by which build, from where"
	// is settled.
	if len(cat) < 6 {
		t.Fatalf("catalog is unexpectedly short: %d", len(cat))
	}
	for i, want := range []string{"host.build", "host.runtime", "host.assets", "host.asset-contains", "host.embedded", "host.embedded-contains"} {
		if cat[i].Name != want {
			t.Errorf("catalog[%d] = %q, want %q (host.* must lead the catalog)", i, cat[i].Name, want)
		}
	}
}

// TestAssetDiagnosticsRunLive calls each through RunDiagnostic exactly as the
// route does. The point is not the content — a test binary's working directory
// has no static root and no provider is wired — but that every degraded path
// returns markdown instead of panicking, which is the state a misconfigured
// deployment is in and the one where the diagnostic must still answer.
func TestAssetDiagnosticsRunLive(t *testing.T) {
	prev := embeddedAssetsFn
	embeddedAssetsFn = nil
	defer func() { embeddedAssetsFn = prev }()

	cat := diagnosticCatalog()
	for _, tc := range []struct{ name, arg string }{
		{"host.assets", ""},
		{"host.assets", "css/"},
		{"host.asset-contains", ""},
		{"host.asset-contains", "../../etc/passwd:root"},
		{"host.embedded", ""},
		{"host.embedded-contains", "calendar:js/x.js:marker"},
	} {
		out, ok := RunDiagnostic(cat, tc.name, tc.arg)
		if !ok {
			t.Fatalf("%s not dispatchable", tc.name)
		}
		if !strings.HasPrefix(out, "## "+tc.name) {
			t.Errorf("%s %q did not produce its own markdown heading:\n%s", tc.name, tc.arg, out)
		}
	}
}
