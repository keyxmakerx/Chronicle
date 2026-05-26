package foundry_vtt

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// TestAdminPackageActionsFragment_RendersAPIMonitorLink pins the
// fragment's output: contains an API monitor link with the right href.
// If the contents shift (e.g. a future contributor moves the link),
// the test surfaces the change.
//
// Per cordinator/decisions/2026-05-23-packages-treatment.md (NW-2.2 Chunk G).
func TestAdminPackageActionsFragment_RendersAPIMonitorLink(t *testing.T) {
	pkg := packages.Package{
		ID:   "fvtt-1",
		Type: packages.PackageTypeFoundryModule,
		Name: "Chronicle-Foundry-Module",
	}

	var buf bytes.Buffer
	if err := AdminPackageActionsFragment(pkg, "test-csrf").Render(context.Background(), &buf); err != nil {
		t.Fatalf("fragment render failed: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `href="/admin/api"`) {
		t.Errorf("fragment output missing API monitor link href:\n%s", out)
	}
	if !strings.Contains(out, `title="API monitor"`) {
		t.Errorf("fragment output missing API monitor title:\n%s", out)
	}
	if !strings.Contains(out, "fa-chart-line") {
		t.Errorf("fragment output missing chart-line icon (API monitor indicator):\n%s", out)
	}
}

// TestAdminPackageActionsFragment_NoOrphanedVersionsButton pins that
// the fragment does NOT render a Versions button. The Versions button
// is generic (lives in packages.templ); the fragment moving it in
// would create a duplicate-button hazard. Documented in the decision
// doc's "deferred to G2" section.
func TestAdminPackageActionsFragment_NoOrphanedVersionsButton(t *testing.T) {
	pkg := packages.Package{ID: "fvtt-1", Type: packages.PackageTypeFoundryModule}

	var buf bytes.Buffer
	if err := AdminPackageActionsFragment(pkg, "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("fragment render failed: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "Versions") {
		t.Errorf("fragment should NOT render a Versions button (packages.templ owns it):\n%s", out)
	}
	if strings.Contains(out, "data-fvtt-versions-trigger") {
		t.Errorf("fragment should NOT carry the versions-trigger data-attr (the attribute lives on packages.templ's generic Versions button as a documented G2 residual):\n%s", out)
	}
}
