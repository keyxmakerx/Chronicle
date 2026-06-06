// entity_page_block_guard_test.go — C-CAL-ENTITY-PAGE-EMBED bug fixes.
// BUG FIX 1: the page-template palette excludes dashboard-only blocks.
// BUG FIX 2: a registered dashboard-only / nil-renderer block on an entity
// page renders a clear placeholder, never a silent blank.
package entities

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestTemplateContextExcludesDashboardOnly — BUG FIX 1 (server side): the
// "template" context filter drops dashboard-only blocks while keeping
// template-context ones. (The client fix is template_editor.js sending
// ?context=template — asserted separately below.)
func TestTemplateContextExcludesDashboardOnly(t *testing.T) {
	reg := NewBlockRegistry()
	reg.Register(BlockMeta{Type: "calendar_preview", Contexts: []string{"dashboard"}}, nil)
	reg.Register(BlockMeta{Type: "entity_calendar", Contexts: []string{"template"}}, func(BlockRenderContext) templ.Component { return templ.NopComponent })
	reg.Register(BlockMeta{Type: "title", Contexts: []string{"template"}}, func(BlockRenderContext) templ.Component { return templ.NopComponent })

	got := map[string]bool{}
	for _, m := range reg.TypesForCampaignAndContext(context.Background(), "camp-1", nil, "template") {
		got[m.Type] = true
	}
	if got["calendar_preview"] {
		t.Errorf("dashboard-only calendar_preview must not appear in the template palette")
	}
	if !got["entity_calendar"] || !got["title"] {
		t.Errorf("template-context blocks must appear in the template palette: %v", got)
	}
}

// TestTemplateEditorRequestsTemplateContext — BUG FIX 1 (client side): the
// page-template editor must request the palette with context=template so the
// server filter actually engages.
func TestTemplateEditorRequestsTemplateContext(t *testing.T) {
	js := readRepoFile(t, "static/js/widgets/template_editor.js")
	if !strings.Contains(js, "entity-types/block-types?context=template") {
		t.Errorf("template_editor.js must fetch block-types with ?context=template (BUG FIX 1)")
	}
}

// TestRenderBlock_PlaceholderNotBlank — BUG FIX 2: a registered block that is
// dashboard-only (wrong context) or nil-renderer renders the placeholder on an
// entity page; a real template block renders normally; an UNREGISTERED type
// still drops silently (intentional).
func TestRenderBlock_PlaceholderNotBlank(t *testing.T) {
	reg := NewBlockRegistry()
	reg.Register(BlockMeta{Type: "calendar_preview", Contexts: []string{"dashboard"}}, nil) // dashboard-only + nil renderer
	reg.Register(BlockMeta{Type: "title", Contexts: []string{"template"}}, func(BlockRenderContext) templ.Component {
		return templ.Raw("<h1>TITLE-OK</h1>")
	})
	SetGlobalBlockRegistry(reg)
	t.Cleanup(func() { SetGlobalBlockRegistry(nil) })

	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}}
	render := func(blockType string) string {
		var sb strings.Builder
		comp := RenderBlock(context.Background(), TemplateBlock{ID: "b1", Type: blockType}, cc, &Entity{ID: "e1"}, &EntityType{ID: 1}, "")
		if err := comp.Render(context.Background(), &sb); err != nil {
			t.Fatalf("render %s: %v", blockType, err)
		}
		return sb.String()
	}

	// Dashboard-only/nil-renderer on a template page → clear placeholder.
	if got := render("calendar_preview"); !strings.Contains(got, "Block not available here") {
		t.Errorf("dashboard-only block should render a placeholder, got: %q", got)
	}
	// Real template block renders normally.
	if got := render("title"); !strings.Contains(got, "TITLE-OK") {
		t.Errorf("template block should render its content, got: %q", got)
	}
	// Unregistered/removed type drops silently (no placeholder noise).
	if got := strings.TrimSpace(render("map_preview")); got != "" {
		t.Errorf("unregistered type should drop silently, got: %q", got)
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}
