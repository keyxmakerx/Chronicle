package systems

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// These tests lead with the answers that MISLEAD, because those are the ones an
// operator acts on. A widget that is present but invisible to `grep`, a plugin
// that is absent from a registry but perfectly present in the build, an unwired
// provider that renders as "nothing here" — each of those is a sentence someone
// would act on wrongly, and each is asserted below by its wording rather than
// by the happy path that would stay green forever while saying it.

// ── registration ───────────────────────────────────────────────────────────

// TestWidgetAndPluginDiagnosticsAreRegistered pins the catalog entries and the
// CLAIMS their descriptions make. The Desc is what the AI assistant reads when
// choosing a diagnostic, so a description that omitted "widgets have no version
// number" would invite it to ask for one.
func TestWidgetAndPluginDiagnosticsAreRegistered(t *testing.T) {
	byName := map[string]Diagnostic{}
	for _, d := range diagnosticCatalog() {
		byName[d.Name] = d
	}

	w, ok := byName["host.widgets"]
	if !ok {
		t.Fatal("host.widgets is not registered in diagnosticCatalog()")
	}
	if w.ArgHint == "" {
		t.Error("host.widgets takes an optional name filter and must advertise it")
	}
	if !strings.Contains(w.Desc, "no version number") && !strings.Contains(w.Desc, "NO version number") {
		t.Errorf("host.widgets' Desc must say widgets carry no version number; got: %s", w.Desc)
	}
	if !strings.Contains(w.Desc, "fingerprint") {
		t.Errorf("host.widgets' Desc must offer the fingerprint as the honest substitute; got: %s", w.Desc)
	}

	p, ok := byName["host.plugins"]
	if !ok {
		t.Fatal("host.plugins is not registered in diagnosticCatalog()")
	}
	if p.ArgHint != "" {
		t.Errorf("host.plugins takes no argument; ArgHint = %q", p.ArgHint)
	}
	if !strings.Contains(p.Desc, "no plugin loader") {
		t.Errorf("host.plugins' Desc must say Chronicle has no plugin loader, or a missing row reads as a missing feature; got: %s", p.Desc)
	}
}

// TestNewDiagnosticsDispatchWithoutPanicking runs each new diagnostic through
// the real dispatcher with the providers unwired — the state a `go test` binary
// is genuinely in — because a nil-deref here would take out the admin page.
func TestNewDiagnosticsDispatchWithoutPanicking(t *testing.T) {
	prevPlugins, prevEmbedded := hostPluginsFn, embeddedAssetsFn
	defer func() { hostPluginsFn, embeddedAssetsFn = prevPlugins, prevEmbedded }()
	hostPluginsFn, embeddedAssetsFn = nil, nil

	for _, tc := range []struct{ name, arg string }{
		{"host.widgets", ""},
		{"host.widgets", "calendar"},
		{"host.plugins", ""},
		{"host.deploy-check", ""},
		{"host.deploy-check", "moonPhase"},
	} {
		out, ok := RunDiagnostic(diagnosticCatalog(), tc.name, tc.arg)
		if !ok {
			t.Fatalf("%s did not dispatch", tc.name)
		}
		if strings.TrimSpace(out) == "" {
			t.Errorf("%s %q produced no output", tc.name, tc.arg)
		}
	}
}

// ── host.plugins ───────────────────────────────────────────────────────────

// TestHostPluginsUnwiredIsNotEmptiness is the load-bearing one. "No plugins are
// registered" and "nothing told this diagnostic about the plugins" are opposite
// conclusions for someone hunting a feature that looks missing, and the whole
// reason this workstream exists is an absence of evidence read as evidence.
func TestHostPluginsUnwiredIsNotEmptiness(t *testing.T) {
	out := renderHostPluginsFrom(nil)
	if !strings.Contains(out, "Provider not wired") {
		t.Errorf("a nil provider must say so; got:\n%s", out)
	}
	if !strings.Contains(out, "not \"no plugins are loaded\"") {
		t.Errorf("the unwired render must explicitly deny the 'no plugins' reading; got:\n%s", out)
	}

	empty := renderHostPluginsFrom(func() []HostPlugin { return nil })
	if strings.Contains(empty, "Provider not wired") {
		t.Errorf("a wired-but-empty provider must NOT render as unwired; got:\n%s", empty)
	}
	if !strings.Contains(empty, "IS an answer") {
		t.Errorf("a wired-but-empty provider must say its emptiness is an answer; got:\n%s", empty)
	}
}

// TestHostPluginsRows drives the states a real deployment can be in. Each case
// asserts on the WORDING, not just on presence: "contributed, but no record"
// and "unhealthy" are different findings and must not print the same.
func TestHostPluginsRows(t *testing.T) {
	fsys := fstest.MapFS{
		"js/gm_panel.js":   &fstest.MapFile{Data: []byte("console.log('gm')")},
		"css/gm_panel.css": &fstest.MapFile{Data: []byte(".gm{}")},
	}

	cases := []struct {
		name    string
		plugin  HostPlugin
		want    []string
		notWant []string
	}{
		{
			name: "static mount is counted and sized",
			plugin: HostPlugin{
				Slug: "calendar", InMetadataRegistry: true, HasHealthCheck: true,
				StaticURLPrefix: "/static/plugins/calendar/", StaticFS: fsys,
			},
			want: []string{"2 file(s)", "/static/plugins/calendar/", "host.embedded calendar", "healthy"},
		},
		{
			name: "a claimed prefix with no filesystem is a defect, not a blank",
			plugin: HostPlugin{
				Slug: "broken", InMetadataRegistry: true,
				StaticURLPrefix: "/static/plugins/broken/",
			},
			want: []string{"nil filesystem", "defect worth reporting"},
		},
		{
			name:   "no static assets is stated, not omitted",
			plugin: HostPlugin{Slug: "smtp", InMetadataRegistry: true},
			want:   []string{"embedded static assets: none registered"},
		},
		{
			name:   "absence from the metadata registry is explicitly not absence from the build",
			plugin: HostPlugin{Slug: "maps", DeclaresMigrations: true, SchemaKnown: true, SchemaHealthy: true, SchemaVersion: 3, SchemaLatest: 3},
			want:   []string{"not registered", "Routes are registered unconditionally", "applied version **3** of 3"},
		},
		{
			name:   "migrations contributed but never observed is neither pass nor fail",
			plugin: HostPlugin{Slug: "timeline", DeclaresMigrations: true},
			want:   []string{"no record for it", "not \"they failed\""},
		},
		{
			name:   "a binary newer than its database is called out",
			plugin: HostPlugin{Slug: "syncapi", DeclaresMigrations: true, SchemaKnown: true, SchemaHealthy: true, SchemaVersion: 2, SchemaLatest: 5},
			want:   []string{"3 migration(s) shipped in this binary have NOT been applied", "newer than the database"},
		},
		{
			name:   "an unhealthy schema carries its error",
			plugin: HostPlugin{Slug: "bestiary", DeclaresMigrations: true, SchemaKnown: true, SchemaVersion: 1, SchemaLatest: 4, SchemaError: "table x\nmissing"},
			want:   []string{"UNHEALTHY", "table x missing"},
			// The applied<latest warning is for a HEALTHY plugin only: on an
			// unhealthy one it would restate the failure as if it were a
			// separate finding.
			notWant: []string{"have NOT been applied"},
		},
		{
			name:   "the two spellings of one plugin are both shown",
			plugin: HostPlugin{Slug: "foundry-vtt", SchemaKey: "foundry_vtt", InMetadataRegistry: true, DeclaresMigrations: true, SchemaKnown: true, SchemaHealthy: true},
			want:   []string{"also spelled `foundry_vtt`", "a grep for one will miss the other"},
		},
		{
			name:   "a failing health callback is reported",
			plugin: HostPlugin{Slug: "calendar", InMetadataRegistry: true, HasHealthCheck: true, HealthErr: "calendar schema unhealthy"},
			want:   []string{"health callback reports: **calendar schema unhealthy**"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := renderHostPluginsFrom(func() []HostPlugin { return []HostPlugin{tc.plugin} })
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("output missing %q:\n%s", w, out)
				}
			}
			for _, n := range tc.notWant {
				if strings.Contains(out, n) {
					t.Errorf("output should not contain %q:\n%s", n, out)
				}
			}
		})
	}
}

// TestHostPluginsNoteIsUnconditional pins that the "a missing row is not a
// missing feature" caveat is printed on a clean run too. A caveat that only
// appears when something is wrong teaches nothing on the run where the answer
// is "not this" — which is most runs.
func TestHostPluginsNoteIsUnconditional(t *testing.T) {
	out := renderHostPluginsFrom(func() []HostPlugin {
		return []HostPlugin{{Slug: "calendar", InMetadataRegistry: true}}
	})
	for _, want := range []string{
		"Chronicle has no plugin loader",
		"absence is normal",
		"source tree cannot be enumerated",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the standing note must contain %q on every run; got:\n%s", want, out)
		}
	}
}

// ── host.widgets ───────────────────────────────────────────────────────────

// widgetTestRoot builds an on-disk static root shaped like the real one.
func widgetTestRoot(t *testing.T) staticRoot {
	t.Helper()
	root := t.TempDir()
	for rel, body := range map[string]string{
		"js/widgets/notes.js":     "Chronicle.register('notes')",
		"js/widgets/inventory.js": "Chronicle.register('inventory')",
		"css/notes.css":           ".notes{}",
		"js/boot.js":              "// not a widget",
	} {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return staticRoot{CWD: filepath.Dir(root), Path: root, Exists: true}
}

// calendarLikeSet mirrors the real calendar plugin: widget scripts under js/
// and one same-named stylesheet under css/.
func calendarLikeSet() []EmbeddedAssetSet {
	return []EmbeddedAssetSet{{
		Slug:      "calendar",
		URLPrefix: "/static/plugins/calendar/",
		FS: fstest.MapFS{
			"js/calendar_widget.js": &fstest.MapFile{Data: []byte("// calendar widget")},
			"js/gm_panel.js":        &fstest.MapFile{Data: []byte("// gm panel")},
			"css/gm_panel.css":      &fstest.MapFile{Data: []byte(".gm{}")},
			"css/orphan.css":        &fstest.MapFile{Data: []byte(".orphan{}")},
		},
	}}
}

// TestHostWidgetsListsBothStorageMechanisms is the diagnostic's reason for
// existing: the on-disk widgets and the widgets compiled into the binary are
// two different inventories, and the calendar widget is only ever in the second.
func TestHostWidgetsListsBothStorageMechanisms(t *testing.T) {
	out := renderHostWidgetsFrom(widgetTestRoot(t), func() []EmbeddedAssetSet { return calendarLikeSet() },
		"", func(u string) string { return u + "?v=deadbeef01" }, "buildtok01")

	for _, want := range []string{
		"**`notes`** — on-disk",
		"**`inventory`** — on-disk",
		"**`calendar_widget`** — embedded in `calendar`",
		"**`gm_panel`** — embedded in `calendar`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// js/boot.js is not under js/widgets/ and must not be claimed as a widget.
	if strings.Contains(out, "**`boot`**") {
		t.Errorf("boot.js is not a widget and must not be listed:\n%s", out)
	}
	// A stylesheet with no same-named script belongs to no widget.
	if strings.Contains(out, "orphan") {
		t.Errorf("css/orphan.css has no matching script and must not appear:\n%s", out)
	}
}

// TestHostWidgetsCompanionAssetsAreListedSeparately pins that a widget's CSS is
// reported as its own line. Folding it into the script's fingerprint would let
// a deploy that changed only the stylesheet look like a no-op.
func TestHostWidgetsCompanionAssetsAreListedSeparately(t *testing.T) {
	out := renderHostWidgetsFrom(widgetTestRoot(t), func() []EmbeddedAssetSet { return calendarLikeSet() },
		"", func(u string) string { return u + "?v=deadbeef01" }, "buildtok01")
	if !strings.Contains(out, "companion `css/gm_panel.css`") {
		t.Errorf("the embedded companion stylesheet must get its own line:\n%s", out)
	}
	if !strings.Contains(out, "companion `css/notes.css`") {
		t.Errorf("the on-disk companion stylesheet must get its own line:\n%s", out)
	}
}

// TestHostWidgetsEmbeddedMtimeIsNotFabricated pins that an embedded asset says
// it has no mtime rather than printing embed.FS's zero time, which would render
// as a confident "0001-01-01".
func TestHostWidgetsEmbeddedMtimeIsNotFabricated(t *testing.T) {
	out := renderHostWidgetsFrom(staticRoot{CWDErr: "no cwd"}, func() []EmbeddedAssetSet { return calendarLikeSet() },
		"", func(u string) string { return u + "?v=deadbeef01" }, "buildtok01")
	if !strings.Contains(out, "mtime n/a") {
		t.Errorf("embedded assets must state that no mtime exists:\n%s", out)
	}
	if strings.Contains(out, "0001-01-01") {
		t.Errorf("the zero time must never be printed as data:\n%s", out)
	}
}

// TestHostWidgetsUnreadableScopesAreNotEmptyResults pins the two degraded
// scopes. Both must explain themselves; neither may render as "there are none".
func TestHostWidgetsUnreadableScopesAreNotEmptyResults(t *testing.T) {
	missingRoot := renderHostWidgetsFrom(staticRoot{CWD: "/x", Path: "/x/static", StatErr: "no such file or directory"},
		func() []EmbeddedAssetSet { return calendarLikeSet() }, "", nil, "buildtok01")
	if !strings.Contains(missingRoot, "is not usable") {
		t.Errorf("an unusable static root must be reported as such:\n%s", missingRoot)
	}
	if !strings.Contains(missingRoot, "falling back to the per-build token") {
		t.Errorf("an unusable static root is itself the explanation for a broken deploy and must say so:\n%s", missingRoot)
	}

	unwired := renderHostWidgetsFrom(widgetTestRoot(t), nil, "", nil, "buildtok01")
	if !strings.Contains(unwired, "Provider not wired") {
		t.Errorf("an unwired embedded provider must say so:\n%s", unwired)
	}
	if !strings.Contains(unwired, "not \"there are none\"") {
		t.Errorf("the unwired render must deny the 'there are none' reading:\n%s", unwired)
	}
}

// TestHostWidgetsFilterDoesNotTakeBlameForAnUnreadScope: when BOTH scopes
// failed to be read, a filter that matched nothing must not be reported as the
// reason. That would be the same absence-of-evidence error in miniature.
func TestHostWidgetsFilterDoesNotTakeBlameForAnUnreadScope(t *testing.T) {
	out := renderHostWidgetsFrom(staticRoot{CWDErr: "getwd failed"}, nil, "calendar", nil, "buildtok01")
	if strings.Contains(out, "No widget name contains") {
		t.Errorf("nothing was scanned, so the filter must not be blamed:\n%s", out)
	}

	scanned := renderHostWidgetsFrom(widgetTestRoot(t), func() []EmbeddedAssetSet { return calendarLikeSet() },
		"nosuchwidget", func(u string) string { return u + "?v=deadbeef01" }, "buildtok01")
	if !strings.Contains(scanned, "No widget name contains") {
		t.Errorf("a genuine no-match after a real scan must say so:\n%s", scanned)
	}
}

// TestHostWidgetsFilterNarrows confirms the filter selects rather than merely
// annotating — and that it selects across BOTH scopes.
func TestHostWidgetsFilterNarrows(t *testing.T) {
	out := renderHostWidgetsFrom(widgetTestRoot(t), func() []EmbeddedAssetSet { return calendarLikeSet() },
		"calendar", func(u string) string { return u + "?v=deadbeef01" }, "buildtok01")
	if !strings.Contains(out, "**`calendar_widget`**") {
		t.Errorf("the filter dropped the widget it should have kept:\n%s", out)
	}
	if strings.Contains(out, "**`notes`**") || strings.Contains(out, "**`gm_panel`**") {
		t.Errorf("the filter kept widgets whose names do not contain it:\n%s", out)
	}
}

// TestHostWidgetsTokenVerdicts pins that the served `?v=` is compared against
// the bytes. A widget whose token no longer matches its file is the single most
// actionable row this diagnostic can produce, and it is unreachable through the
// live AssetURL because digests are memoised for the process lifetime.
func TestHostWidgetsTokenVerdicts(t *testing.T) {
	root := widgetTestRoot(t)

	stale := renderHostWidgetsFrom(root, nil, "notes", func(u string) string { return u + "?v=0000000000" }, "buildtok01")
	if !strings.Contains(stale, "STALE") {
		t.Errorf("a token that does not match the bytes must be flagged STALE:\n%s", stale)
	}

	fallback := renderHostWidgetsFrom(root, nil, "notes", func(u string) string { return u + "?v=buildtok01" }, "buildtok01")
	if !strings.Contains(fallback, "BUILD-TOKEN FALLBACK") {
		t.Errorf("the per-build token must be flagged as a fallback:\n%s", fallback)
	}
}

// TestHostWidgetsNoteStatesItsOwnLimits pins the three sentences a reader would
// otherwise supply wrongly from their own assumptions: that this walked
// directories because no registry exists, that embedded bytes are invisible to
// grep, and that the Go widget packages cannot be enumerated from here.
func TestHostWidgetsNoteStatesItsOwnLimits(t *testing.T) {
	out := renderHostWidgetsFrom(widgetTestRoot(t), func() []EmbeddedAssetSet { return calendarLikeSet() },
		"", func(u string) string { return u + "?v=deadbeef01" }, "buildtok01")
	for _, want := range []string{
		"no widget registry to consult",
		"WALKED two known directories",
		"never find them",
		"The Go widget packages under `internal/widgets/` are not listed and cannot be",
		"limit of the vantage point, not a finding",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the standing note must contain %q; got:\n%s", want, out)
		}
	}
	// The header claim is the one the whole diagnostic rests on: an operator who
	// came looking for a version number has to be told there is none, not left
	// to read a hash as though it were one.
	if !strings.Contains(out, "**Widgets carry no version number.**") {
		t.Errorf("the header must state outright that widgets carry no version number:\n%s", out)
	}
}

// TestCountEmbeddedFiles pins the count/size used by host.plugins against a
// filesystem whose contents are known exactly.
func TestCountEmbeddedFiles(t *testing.T) {
	files, total, walkErr := countEmbeddedFiles(fstest.MapFS{
		"js/a.js":   &fstest.MapFile{Data: []byte("12345")},
		"css/b.css": &fstest.MapFile{Data: []byte("123")},
	})
	if walkErr != "" {
		t.Fatalf("unexpected walk error: %s", walkErr)
	}
	if files != 2 || total != 8 {
		t.Errorf("files=%d total=%d, want 2 and 8", files, total)
	}
	if files, total, _ := countEmbeddedFiles(fstest.MapFS{}); files != 0 || total != 0 {
		t.Errorf("an empty filesystem must count zero, got %d/%d", files, total)
	}
}

// TestWidgetModTime pins the two renderings apart.
func TestWidgetModTime(t *testing.T) {
	ts := time.Date(2026, 8, 11, 3, 4, 5, 0, time.UTC)
	if got := widgetModTime(widgetAsset{ModTime: ts, HasMod: true}); got != "2026-08-11T03:04:05Z" {
		t.Errorf("on-disk mtime = %q", got)
	}
	if got := widgetModTime(widgetAsset{}); !strings.Contains(got, "n/a") {
		t.Errorf("an embedded asset must report no mtime, got %q", got)
	}
}
