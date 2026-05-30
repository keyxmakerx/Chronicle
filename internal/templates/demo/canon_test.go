// canon_test.go — C-V2-DESIGN-REBUILD Phase 2A canon-demo tests.
//
// Pinned invariants:
//
//   1. Render smoke + the old PR #375-#381 demo files + route are gone.
//   2. Tabbed shell: Buttons active by default; Menus/Cards/Forms/
//      Calendar/Timeline render as disabled labels with hints.
//   3. Buttons tab content: all 7 sections (C.1-C.7) present and
//      carrying the data hooks the JS init blocks bind.
//   4. JS architecture: external file referenced via <script src=…>;
//      INIT_BLOCKS registry; per-block try/catch; new picks-store key
//      `chronicle-canon-picks-v2`; document.title fallback.
//   5. CSS hard-rule lint guards: no `transition: all`, no hex literals
//      outside id selectors, no shadow-alone transitions.

package demo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// ============================================================
// 1. Render + scrap pins
// ============================================================

func TestDemoCanon_RendersWithoutPanic(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if buf.Len() < 1000 {
		t.Errorf("render too small (%d bytes)", buf.Len())
	}
	html := buf.String()
	if !strings.Contains(html, `data-chronicle-canon`) {
		t.Errorf("canon root scope attribute missing")
	}
}

// TestDemoCanon_OldFilesDeleted — the Phase 2A scrap must actually
// remove the PR #375-#381 demo files. If a future "let me migrate
// back" edit reintroduces them they'll collide; pin the deletion.
func TestDemoCanon_OldFilesDeleted(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	scrapped := []string{
		"internal/templates/demo/chronicle_canon.templ",
		"internal/templates/demo/chronicle_canon_test.go",
		"static/css/chronicle-canon-demo.css",
		"static/js/chronicle-canon-demo.js",
	}
	for _, rel := range scrapped {
		p := filepath.Join(repoRoot, rel)
		if _, err := os.Stat(p); err == nil {
			t.Errorf("scrapped file still exists: %s (Phase 2A deleted this; if you need to bring it back, that's a separate dispatch)", rel)
		}
	}
}

// TestDemoCanon_OldRouteRemoved — the old `GET /demo/chronicle-canon`
// route is gone; new `GET /demo/canon` is registered. Verified via
// the wire snapshot (the route presence-test surface).
func TestDemoCanon_OldRouteRemoved(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	snap, err := os.ReadFile(filepath.Join(repoRoot, "internal", "wire", "routes_snapshot.txt"))
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	s := string(snap)
	if strings.Contains(s, "/demo/chronicle-canon") {
		t.Errorf("wire snapshot still references the scrapped /demo/chronicle-canon route")
	}
	if !strings.Contains(s, "/demo/canon") {
		t.Errorf("wire snapshot missing new /demo/canon route — regenerate")
	}
}

// ============================================================
// 2. Tabbed shell
// ============================================================

func TestDemoCanon_ButtonsTabActive(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, m := range []string{
		`data-active-tab="buttons"`,
		`data-tab="buttons"`,
		`chronicle-canon__tab--active`,
		`id="panel-buttons"`,
		`Buttons`,
	} {
		if !strings.Contains(html, m) {
			t.Errorf("active Buttons tab marker missing: %s", m)
		}
	}
}

func TestDemoCanon_OtherTabsDisabled(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, tab := range []string{"menus", "cards", "forms", "calendar", "timeline"} {
		marker := `data-tab="` + tab + `"`
		if !strings.Contains(html, marker) {
			t.Errorf("disabled tab missing: %s", tab)
		}
	}
	// Each disabled tab must carry the disabled attribute hook + a
	// hint label so operator sees this is intentional.
	if !strings.Contains(html, `data-tab-disabled`) {
		t.Errorf("disabled-tab hook missing")
	}
	if !strings.Contains(html, `Coming next phase`) && !strings.Contains(html, `Coming in phase`) {
		t.Errorf("disabled-tab hint copy missing — operator needs to know these are intentional, not broken")
	}
}

// ============================================================
// 3. Buttons tab — all 7 sections + data hooks
// ============================================================

func TestDemoCanon_ButtonsTabSections(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, id := range []string{
		`id="c1-variants"`,
		`id="c2-sizes"`,
		`id="c3-states"`,
		`id="c4-shadows"`,
		`id="c5-hover"`,
		`id="c6-motion"`,
		`id="c7-selected"`,
	} {
		if !strings.Contains(html, id) {
			t.Errorf("Buttons tab section missing: %s", id)
		}
	}
}

func TestDemoCanon_PickHooks(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, m := range []string{
		`data-pick`,
		`data-pick-axis="variant"`,
		`data-pick-axis="size"`,
		`data-pick-axis="shadow"`,
		`data-pick-axis="hover"`,
		`data-pick-axis="motion"`,
		`data-pick-axis="selected"`,
	} {
		if !strings.Contains(html, m) {
			t.Errorf("pick-hook missing: %s", m)
		}
	}
	// Operator's binding goal: many options per axis. Pin minimum
	// instance counts so a future shrink-too-far edit fails loudly.
	// C.1: 6 families × 2 sub-variants = 12 instances.
	pairs := strings.Count(html, `chronicle-canon__variant-family`)
	if pairs < 6 {
		t.Errorf("Variants (C.1) should render at least 6 families; got %d", pairs)
	}
	// C.2 sizes: 5 cards.
	sizes := strings.Count(html, `data-pick-axis="size"`)
	if sizes < 5 {
		t.Errorf("Sizes (C.2) should render at least 5 cards; got %d", sizes)
	}
	// C.4 shadows: 4 cards. C.5 hover: 4 cards. C.6 motion: 3 cards.
	if c := strings.Count(html, `data-pick-axis="shadow"`); c < 4 {
		t.Errorf("Shadows (C.4) should render at least 4 cards; got %d", c)
	}
	if c := strings.Count(html, `data-pick-axis="hover"`); c < 4 {
		t.Errorf("Hover animations (C.5) should render at least 4 cards; got %d", c)
	}
	if c := strings.Count(html, `data-pick-axis="motion"`); c < 3 {
		t.Errorf("Motion timings (C.6) should render at least 3 cards; got %d", c)
	}
	// Per-axis data attributes on the sample buttons so the CSS per-
	// axis variants engage.
	for _, m := range []string{`data-shadow="subtle"`, `data-hover="lift"`, `data-motion="canon"`, `data-selected="A"`} {
		if !strings.Contains(html, m) {
			t.Errorf("sample-button axis attribute missing: %s", m)
		}
	}
}

func TestDemoCanon_PicksPanelMarkup(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, m := range []string{
		`data-picks-panel`,
		`data-picks-count`,
		`data-picks-tabname`,
		`data-picks-list`,
		`data-picks-empty`,
		`data-picks-brief`,
		`data-picks-copy`,
		`data-picks-download`,
		`data-picks-reset`,
		`data-notes-tab="buttons"`,
	} {
		if !strings.Contains(html, m) {
			t.Errorf("picks-panel marker missing: %s", m)
		}
	}
}

// ============================================================
// 4. JS architecture
// ============================================================

func TestDemoCanonJS_ExternalFileLoaded(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, `src="/static/js/chronicle-canon.js"`) {
		t.Errorf("demo templ must load /static/js/chronicle-canon.js via <script src=… defer>")
	}
	if !strings.Contains(html, "defer") {
		t.Errorf("external script tag must carry defer")
	}
	if _, err := os.Stat(canonJSPath(t)); err != nil {
		t.Errorf("external JS file missing on disk: %v", err)
	}
}

func TestDemoCanonJS_NoInlineScriptBlocks(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	templPath := filepath.Join(filepath.Dir(thisFile), "canon.templ")
	b, err := os.ReadFile(templPath)
	if err != nil {
		t.Fatalf("read canon templ: %v", err)
	}
	src := string(b)
	// Strip Go-line comments so file-header narration that mentions
	// "no inline script" doesn't trigger the lint.
	src = regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(src, "")
	open := regexp.MustCompile(`(?i)<script\b([^>]*)>`)
	for _, m := range open.FindAllStringSubmatch(src, -1) {
		attrs := strings.ToLower(m[1])
		if strings.Contains(attrs, "src=") {
			continue
		}
		t.Errorf("inline <script> tag in canon templ; forbidden: %q", m[1])
	}
}

func TestDemoCanonJS_AllInitBlocksRegistered(t *testing.T) {
	js := readCanonJS(t)
	required := []string{
		"diagnostic-dashboard",
		"browser-compat-detect",
		"theme-toggle",
		"reduced-motion-toggle",
		"tab-strip",
		"picks-hydrate-and-bind",
		"notes-bind",
		"picks-panel",
		"diagnostics-copy-report",
	}
	for _, name := range required {
		marker := "registerInitBlock('" + name + "'"
		if !strings.Contains(js, marker) {
			t.Errorf("init block not registered: %s", name)
		}
	}
	if !strings.Contains(js, "INIT_BLOCKS") {
		t.Errorf("INIT_BLOCKS registry missing")
	}
	if !strings.Contains(js, "try {") || !strings.Contains(js, "catch (err)") {
		t.Errorf("per-block try/catch missing")
	}
	if !strings.Contains(js, "window.__chronicleCanonInited = true") {
		t.Errorf("__chronicleCanonInited assignment missing")
	}
	if !strings.Contains(js, "document.title") {
		t.Errorf("document.title fallback missing")
	}
	// New picks key per dispatch — must not migrate from old demo.
	if !strings.Contains(js, "'chronicle-canon-picks-v2'") {
		t.Errorf("new picks key 'chronicle-canon-picks-v2' missing — Phase 2A starts fresh, no migration")
	}
	// Confirm no carryover to the old key (would silently revive
	// scrapped data; dispatch says no migration).
	if strings.Contains(js, "chronicle-canon-decisions") {
		t.Errorf("old chronicle-canon-decisions key should not appear in fresh implementation (no migration per dispatch)")
	}
}

func TestDemoCanon_DiagnosticDashboardPresent(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, m := range []string{
		`data-canon-diagnostics`,
		`Demo Diagnostics`,
		`data-init-block-list`,
		`data-binding-list`,
		`data-feature-list`,
		`data-ua-string`,
		`data-last-action-log`,
		`data-copy-report`,
		`data-diagnostics-status`,
	} {
		if !strings.Contains(html, m) {
			t.Errorf("diagnostic dashboard marker missing: %s", m)
		}
	}
	// Stop-and-flag #8: collapsed-by-default must not hide failures.
	// JS auto-expands via setAttribute('open', '') if any block failed.
	js := readCanonJS(t)
	if !strings.Contains(js, "panel.setAttribute('open', '')") {
		t.Errorf("JS must auto-expand the <details> dashboard when blocks fail (stop-and-flag #8)")
	}
}

// ============================================================
// 5. CSS hard-rule lint guards
// ============================================================

func TestCanonCSS_NoTransitionAll(t *testing.T) {
	src := stripCSSComments(readCanonCSS(t))
	forbidden := "trans" + "ition: all"
	if strings.Contains(src, forbidden) {
		t.Errorf("chronicle-canon.css contains forbidden `transition: all` — canon D5")
	}
}

func TestCanonCSS_NoHexLiterals(t *testing.T) {
	src := stripCSSComments(readCanonCSS(t))
	idSelector := regexp.MustCompile(`#[a-zA-Z][\w-]*`)
	scrubbed := idSelector.ReplaceAllString(src, "")
	hex := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	matches := hex.FindAllString(scrubbed, -1)
	if len(matches) > 0 {
		t.Errorf("chronicle-canon.css contains %d hex literal(s) — canon D2 OKLCH-only.\nFirst few: %v",
			len(matches), matches[:min(5, len(matches))])
	}
}

func TestCanonCSS_NoShadowAloneTransition(t *testing.T) {
	src := stripCSSComments(readCanonCSS(t))
	transitionDecl := regexp.MustCompile(`(?s)transition:\s*([^;]+);`)
	for _, m := range transitionDecl.FindAllStringSubmatchIndex(src, -1) {
		body := src[m[2]:m[3]]
		if !strings.Contains(body, "box-shadow") {
			continue
		}
		coOccur := []string{
			"background-color", "border-color", "color",
			"transform", "opacity", "background",
		}
		paired := false
		for _, c := range coOccur {
			if strings.Contains(body, c) {
				paired = true
				break
			}
		}
		if paired {
			continue
		}
		t.Errorf("transition animates box-shadow alone — canon D3.\nDeclaration: %s", strings.TrimSpace(body))
	}
}

func TestCanonCSS_TokenScopedToCanonRoot(t *testing.T) {
	// All --chronicle-* token declarations should live inside a
	// [data-chronicle-canon] selector so they don't leak. Heuristic:
	// the surface area on disk must mention [data-chronicle-canon]
	// before any --chronicle-surface declaration.
	src := readCanonCSS(t)
	scopeIdx := strings.Index(src, "[data-chronicle-canon] {")
	surfaceIdx := strings.Index(src, "--chronicle-surface:")
	if scopeIdx < 0 || surfaceIdx < 0 || scopeIdx > surfaceIdx {
		t.Errorf("--chronicle-* tokens must be declared inside [data-chronicle-canon] {…} (scope-isolation)")
	}
}

// ============================================================
// Helpers
// ============================================================

func stripCSSComments(src string) string {
	return regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(src, "")
}

func canonCSSPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	return filepath.Join(repoRoot, "static", "css", "chronicle-canon.css")
}

func canonJSPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	return filepath.Join(repoRoot, "static", "js", "chronicle-canon.js")
}

func readCanonCSS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(canonCSSPath(t))
	if err != nil {
		t.Fatalf("read canon css: %v", err)
	}
	return string(b)
}

func readCanonJS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(canonJSPath(t))
	if err != nil {
		t.Fatalf("read canon js: %v", err)
	}
	return string(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
