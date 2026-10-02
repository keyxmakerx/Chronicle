package systems

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestSystemManifest_HasWidget checks the lookup SystemIndexContent relies on
// to decide whether to mount the Rulebook front page: present when the
// manifest declares the slug, absent otherwise (no widgets at all, or
// widgets that don't include it).
func TestSystemManifest_HasWidget(t *testing.T) {
	tests := []struct {
		name    string
		widgets []WidgetDef
		slug    string
		want    bool
	}{
		{
			name:    "widget registered",
			widgets: []WidgetDef{{Slug: "rulebook-frontpage", Name: "Rulebook Front Page", ScriptFile: "widgets/rulebook-frontpage.js"}},
			slug:    "rulebook-frontpage",
			want:    true,
		},
		{
			name:    "other widgets, not this one",
			widgets: []WidgetDef{{Slug: "character-sheet", Name: "Character Sheet", ScriptFile: "widgets/character-sheet.js"}},
			slug:    "rulebook-frontpage",
			want:    false,
		},
		{
			name:    "no widgets at all",
			widgets: nil,
			slug:    "rulebook-frontpage",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &SystemManifest{Widgets: tt.widgets}
			if got := m.HasWidget(tt.slug); got != tt.want {
				t.Errorf("HasWidget(%q) = %v, want %v", tt.slug, got, tt.want)
			}
		})
	}
}

// TestSystemIndexContent_RulebookFrontpageMount renders the reference index
// page for an enabled system that does/doesn't register the
// "rulebook-frontpage" widget. The "no widgets" case also stands in for a
// campaign with no system at all: this route only renders once a system is
// resolved (requireSystemAddon in routes.go gates it), so "no widgets
// declared" is the closest reachable analogue of "nothing to mount" this
// templ can express on its own. Asserts the mount div and its campaign-id
// config are present or absent accordingly, and that the generic category
// browser renders only when there is no front page to stand in for it.
// When it mounts, it is wrapped full-bleed with no breadcrumb.
func TestSystemIndexContent_RulebookFrontpageMount(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test Campaign"}}
	cats := []categoryInfo{{Slug: "abilities", Name: "Abilities", Count: 3}}

	tests := []struct {
		name      string
		manifest  *SystemManifest
		wantMount bool
	}{
		{
			name: "enabled system registers rulebook-frontpage",
			manifest: &SystemManifest{
				ID:   "drawsteel",
				Name: "Draw Steel",
				Widgets: []WidgetDef{
					{Slug: "rulebook-frontpage", Name: "Rulebook Front Page", ScriptFile: "widgets/rulebook-frontpage.js"},
				},
			},
			wantMount: true,
		},
		{
			name: "enabled system does not register it",
			manifest: &SystemManifest{
				ID:   "dnd5e",
				Name: "D&D 5e",
				Widgets: []WidgetDef{
					{Slug: "character-sheet", Name: "Character Sheet", ScriptFile: "widgets/character-sheet.js"},
				},
			},
			wantMount: false,
		},
		{
			name: "system with no widgets (stands in for a campaign with no system)",
			manifest: &SystemManifest{
				ID:   "bare",
				Name: "Bare System",
			},
			wantMount: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := SystemIndexContent(cc, tt.manifest, cats).Render(context.Background(), &buf); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			html := buf.String()

			mountIdx := strings.Index(html, `data-widget="rulebook-frontpage"`)
			gotMount := mountIdx >= 0
			if gotMount != tt.wantMount {
				t.Errorf("mount present = %v, want %v; html:\n%s", gotMount, tt.wantMount, html)
			}
			if tt.wantMount && !strings.Contains(html, `data-campaign-id="camp-1"`) {
				t.Errorf("expected mount to carry data-campaign-id=%q; html:\n%s", "camp-1", html)
			}
			gotBrowser := strings.Contains(html, "Abilities")
			if gotBrowser == tt.wantMount {
				t.Errorf("category browser present = %v, want %v; html:\n%s", gotBrowser, !tt.wantMount, html)
			}
			// The rulebook fills the content area: wrapped full-bleed and with no
			// breadcrumb above it. The category browser keeps its breadcrumb.
			gotFullBleed := strings.Contains(html, "data-rulebook-fullbleed")
			if gotFullBleed != tt.wantMount {
				t.Errorf("full-bleed wrapper present = %v, want %v; html:\n%s", gotFullBleed, tt.wantMount, html)
			}
			gotCrumbs := strings.Contains(html, `aria-label="Breadcrumb"`)
			if gotCrumbs == tt.wantMount {
				t.Errorf("breadcrumb present = %v, want %v; html:\n%s", gotCrumbs, !tt.wantMount, html)
			}
		})
	}
}
