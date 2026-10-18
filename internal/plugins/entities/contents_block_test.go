package entities

import (
	"context"
	"strings"
	"testing"
)

func renderContents(t *testing.T, e *Entity) string {
	t.Helper()
	var sb strings.Builder
	if err := blockContents(e).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

// The Contents block is a core block: always offered, only on the entity
// page, one per page, and not a tile (in the rail it takes no room, so a tile
// would be an empty cell).
func TestContentsBlockIsRegistered(t *testing.T) {
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	var meta *BlockMeta
	for _, m := range reg.Types() {
		if m.Type == ContentsBlockType {
			m := m
			meta = &m
		}
	}
	if meta == nil {
		t.Fatal("the contents block type is not registered")
	}
	tests := []struct {
		name string
		ok   bool
	}{
		{"always available: no add-on", meta.Addon == ""},
		{"entity page only", len(meta.Contexts) == 1 && meta.Contexts[0] == "template"},
		{"one per page", reg.IsSingleton(ContentsBlockType)},
		{"not a tile", len(meta.TileSizes) == 0},
		{"labelled for the Customizer", meta.Label == "Contents" && meta.Icon != ""},
		{"valid in the page context", reg.IsValidForContext(ContentsBlockType, "template")},
		{"not offered on the dashboard", !reg.IsValidForContext(ContentsBlockType, "dashboard")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.ok {
				t.Errorf("block meta = %+v", *meta)
			}
		})
	}
}

// A saved layout may hold the block once; a second one is refused.
func TestContentsBlockSingletonInLayout(t *testing.T) {
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	col := func(id string, n int) TemplateColumn {
		c := TemplateColumn{ID: id, Width: 12}
		for i := 0; i < n; i++ {
			c.Blocks = append(c.Blocks, TemplateBlock{ID: id + "-b" + string(rune('a'+i)), Type: ContentsBlockType})
		}
		return c
	}
	tests := []struct {
		name    string
		n       int
		wantErr bool
	}{
		{"none", 0, false},
		{"one", 1, false},
		{"two", 2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := EntityTypeLayout{Rows: []TemplateRow{{ID: "r1", Columns: []TemplateColumn{col("c1", tt.n)}}}}
			err := ValidateLayout(layout, reg.IsValid, reg.IsSingleton, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLayout error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// The server sends only an empty shell that starts hidden: no heading text
// (headings are read from the viewer's own page in the browser, so nothing a
// viewer may not read can come from here), no script tag (an HTMX swap may
// not run one), and every handler inline.
func TestContentsBlockShell(t *testing.T) {
	out := renderContents(t, &Entity{ID: "e1", Name: "Port Varrow"})
	tests := []struct {
		name string
		has  []string
		not  []string
	}{
		{"starts hidden and mounts the widget", []string{`data-widget="page-contents"`, `data-ct="off"`}, nil},
		{"the rail, the narrow-page button and the panel", []string{`class="ct-rail"`, `class="ct-btn"`, `class="ct-pn"`, `role="region"`}, nil},
		{"labelled for screen readers", []string{`aria-label="Contents"`, `aria-label="Close contents"`, `aria-label="Find a heading"`, `aria-expanded="false"`}, nil},
		{"the icon and the button open the panel for this page", []string{`aria-controls="ct-panel-e1"`, `id="ct-panel-e1"`}, nil},
		{"inline handlers", []string{`window.ChronicleContents.toggle`, `window.ChronicleContents.stripMove`, `window.ChronicleContents.stripKey`, `window.ChronicleContents.close`, `window.ChronicleContents.filter`}, nil},
		{"no script of its own, no page text, no emoji icons", nil, []string{"<script", "Port Varrow", "fa-"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, w := range tt.has {
				if !strings.Contains(out, w) {
					t.Errorf("shell lacks %q", w)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(out, n) {
					t.Errorf("shell contains %q", n)
				}
			}
		})
	}
}

func TestContentsPanelID(t *testing.T) {
	tests := []struct {
		name string
		e    *Entity
		want string
	}{
		{"an entity", &Entity{ID: "abc"}, "ct-panel-abc"},
		{"no entity", nil, "ct-panel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentsPanelID(tt.e); got != tt.want {
				t.Errorf("contentsPanelID = %q, want %q", got, tt.want)
			}
		})
	}
}

// Every handler the shell names must exist in the script, or a click would do
// nothing and say nothing.
func TestContentsHandlersExistInScript(t *testing.T) {
	js, err := StaticAssetsFS.ReadFile("static/js/page_contents.js")
	if err != nil {
		t.Fatal(err)
	}
	out := renderContents(t, &Entity{ID: "e1"})
	const marker = "window.ChronicleContents."
	seen := 0
	for rest := out; ; {
		i := strings.Index(rest, marker)
		if i < 0 {
			break
		}
		rest = rest[i+len(marker):]
		name := rest[:strings.IndexAny(rest, "(")]
		seen++
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(string(js), "\n    "+name+": function") {
				t.Errorf("page_contents.js has no handler %q", name)
			}
		})
	}
	if seen == 0 {
		t.Fatal("the shell names no handlers")
	}
}
