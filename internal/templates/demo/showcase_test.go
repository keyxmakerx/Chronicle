// showcase_test.go — C-V2-DESIGN-REBUILD demo showcase tests.
//
// Light guards — the demo is NOT load-bearing for canon discipline
// (the canon lives in decisions/2026-05-29-chronicle-design-canon.md,
// not in the demo CSS). These pin the scrap-and-rebuild invariants:
// the old instrument arc is gone, the showcase renders, it ships zero
// demo JS, and every option carries a stable label.

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

func TestDemoShowcase_RendersWithoutPanic(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoShowcase().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if buf.Len() < 1000 {
		t.Errorf("render too small (%d bytes)", buf.Len())
	}
	if !strings.Contains(buf.String(), "showcase-page") {
		t.Errorf("showcase root class missing")
	}
}

// TestDemoShowcase_OldRoutesAndFilesGone — the instrument arc (PRs
// #375-#383) is fully scrapped. Pin the deletion so a future "let me
// migrate it back" edit fails loudly, and confirm the wire snapshot
// swapped /demo/canon → /demo/showcase.
func TestDemoShowcase_OldRoutesAndFilesGone(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		"internal/templates/demo/canon.templ",
		"internal/templates/demo/canon_test.go",
		"static/css/chronicle-canon.css",
		"static/js/chronicle-canon.js",
		"internal/templates/demo/chronicle_canon.templ",
		"static/css/chronicle-canon-demo.css",
		"static/js/chronicle-canon-demo.js",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			t.Errorf("scrapped demo file still exists: %s", rel)
		}
	}
	snap, err := os.ReadFile(filepath.Join(root, "internal", "wire", "routes_snapshot.txt"))
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	s := string(snap)
	if strings.Contains(s, "/demo/canon") || strings.Contains(s, "/demo/chronicle-canon") {
		t.Errorf("wire snapshot still references a scrapped demo route")
	}
	if !strings.Contains(s, "/demo/showcase") {
		t.Errorf("wire snapshot missing new /demo/showcase route — regenerate")
	}
}

// TestDemoShowcase_NoDemoJS — the showcase ships ZERO demo JavaScript.
// No <script src=> for any demo-specific file, and no inline <script>.
func TestDemoShowcase_NoDemoJS(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoShowcase().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if strings.Contains(html, "chronicle-canon.js") || strings.Contains(html, "chronicle-canon-demo.js") {
		t.Errorf("showcase must not reference any scrapped demo JS")
	}
	// No inline <script> body and no <script src> introduced by the
	// showcase templ itself. (We scan the templ source so layout-level
	// scripts from base.templ don't false-positive.)
	_, thisFile, _, _ := runtime.Caller(0)
	templSrc, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "showcase.templ"))
	if err != nil {
		t.Fatalf("read templ: %v", err)
	}
	// Strip Go line-comments so the file-header narration (which
	// legitimately says "no <script> tag") doesn't false-positive.
	stripped := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(string(templSrc), "")
	if regexp.MustCompile(`(?i)<script`).MatchString(stripped) {
		t.Errorf("showcase.templ must contain no <script> tag — the showcase is pure no-JS markup")
	}
}

// TestDemoShowcase_LabelsPresent — every showcase axis renders labeled
// options the operator references in chat. Pin the stable label IDs.
func TestDemoShowcase_LabelsPresent(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoShowcase().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	// At least one label from each axis must be present.
	for _, id := range []string{"V1", "V6", "S1", "S5", "SH1", "SH4", "HA1", "HA4", "MT1", "MT3", "SS1", "SS2", "AC1", "AC4", "BG1", "BG4"} {
		if !strings.Contains(html, ">"+id+"<") {
			t.Errorf("showcase label %q missing", id)
		}
	}
	if c := strings.Count(html, "showcase-label"); c < 30 {
		t.Errorf("expected 30+ labeled options across all axes; got %d", c)
	}
}

// TestDemoShowcase_AxisCount — all 8 axes render as sections.
func TestDemoShowcase_AxisCount(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoShowcase().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if c := strings.Count(html, "showcase-section__title"); c != 8 {
		t.Errorf("expected exactly 8 showcase axes; got %d", c)
	}
}

// TestDemoShowcase_CSSSelfContained — the showcase CSS must not depend
// on Tailwind or the prior canon tokens (the whole point of the reset:
// no cascade fight). No @layer, no @apply, no --chronicle-* tokens.
func TestDemoShowcase_CSSSelfContained(t *testing.T) {
	root := repoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "static", "css", "showcase.css"))
	if err != nil {
		t.Fatalf("read showcase.css: %v", err)
	}
	// Strip /* ... */ comments first — the file header legitimately
	// documents "no @layer, no @apply, no --chronicle-* tokens".
	css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(string(b), "")
	for _, forbidden := range []string{"@layer", "@apply", "--chronicle-"} {
		if strings.Contains(css, forbidden) {
			t.Errorf("showcase.css must not contain %q (self-contained, no cascade fight)", forbidden)
		}
	}
	// Must carry real hover rules (the dead-hover regression guard).
	if !strings.Contains(css, ":hover") {
		t.Errorf("showcase.css has no :hover rules")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
}
