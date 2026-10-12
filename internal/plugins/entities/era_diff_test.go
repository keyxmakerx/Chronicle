package entities

import (
	"strings"
	"testing"
)

func TestEraDiff(t *testing.T) {
	usual := `<p>Emberfall is a guild city on the slopes of the Cinder Mount.</p>` +
		`<p>Its Glassguild sits in a hall of green crystal.</p>` +
		`<h2>Harbour</h2><p>The harbour takes ships from the whole coast.</p>`
	tests := []struct {
		name    string
		version string
		kinds   []string
		check   func(t *testing.T, d []EraDiffBlock)
	}{
		{
			name:    "identical text changes nothing",
			version: usual,
			kinds:   []string{EraDiffSame, EraDiffSame, EraDiffSame, EraDiffSame},
		},
		{
			name: "an edited paragraph keeps its place and marks the new words",
			version: `<p>Ashgate is a <strong>walled</strong> city on the slopes of the Cinder Mount.</p>` +
				`<p>Its Glassguild sits in a hall of green crystal.</p>` +
				`<h2>Harbour</h2><p>The harbour takes ships from the whole coast.</p>`,
			kinds: []string{EraDiffChanged, EraDiffSame, EraDiffSame, EraDiffSame},
			check: func(t *testing.T, d []EraDiffBlock) {
				h := d[0].HTML
				if !strings.Contains(h, `<mark class="era-w">Ashgate</mark>`) {
					t.Errorf("new word not marked: %s", h)
				}
				if !strings.Contains(h, `<strong><mark class="era-w">walled</mark></strong>`) {
					t.Errorf("formatting lost or word not marked: %s", h)
				}
				if strings.Contains(h, `<mark class="era-w">city</mark>`) {
					t.Errorf("unchanged word marked: %s", h)
				}
			},
		},
		{
			name: "a removed paragraph is kept as the usual text, an added one is added",
			version: `<p>Emberfall is a guild city on the slopes of the Cinder Mount.</p>` +
				`<h2>Harbour</h2><p>The harbour takes ships from the whole coast.</p>` +
				`<p>The Cinder Council took the walls.</p>`,
			kinds: []string{EraDiffSame, EraDiffRemoved, EraDiffSame, EraDiffSame, EraDiffAdded},
			check: func(t *testing.T, d []EraDiffBlock) {
				if !strings.Contains(d[1].HTML, "Glassguild") {
					t.Errorf("removed block should carry the usual text: %s", d[1].HTML)
				}
			},
		},
		{
			name: "an edit just after a removed paragraph is still an edit",
			version: `<p>Emberfall is a guild city on the slopes of the Cinder Mount.</p>` +
				`<h2>Harbour</h2><p>The harbour is choked and takes boats from the whole coast.</p>`,
			kinds: []string{EraDiffSame, EraDiffRemoved, EraDiffSame, EraDiffChanged},
		},
		{
			name: "a removed paragraph before an edited one in the same run",
			version: `<p>Ashgate is a walled city on the slopes of the Cinder Mount.</p>` +
				`<h2>Harbour</h2><p>The harbour takes ships from the whole coast.</p>`,
			kinds: []string{EraDiffChanged, EraDiffRemoved, EraDiffSame, EraDiffSame},
		},
		{
			name:    "a whole new text replaces an unrelated one",
			version: `<p>Nothing remains.</p>`,
			kinds:   []string{EraDiffRemoved, EraDiffRemoved, EraDiffRemoved, EraDiffRemoved, EraDiffAdded},
		},
		{
			name:    "text outside any tag is one block",
			version: `loose <em>words</em> here`,
			check: func(t *testing.T, d []EraDiffBlock) {
				found := false
				for _, b := range d {
					if b.Kind == EraDiffAdded && strings.Contains(b.HTML, "<em>words</em>") {
						found = true
					}
				}
				if !found {
					t.Errorf("loose text not one added block: %+v", d)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := EraDiff(usual, tt.version)
			if tt.kinds != nil {
				var got []string
				for _, b := range d {
					got = append(got, b.Kind)
				}
				if strings.Join(got, ",") != strings.Join(tt.kinds, ",") {
					t.Fatalf("kinds = %v, want %v", got, tt.kinds)
				}
			}
			if tt.check != nil {
				tt.check(t, d)
			}
		})
	}
}

func TestEraDiffEscapesText(t *testing.T) {
	d := EraDiff(`<p>a b</p>`, `<p>a &lt;script&gt; b</p>`)
	if len(d) != 1 || d[0].Kind != EraDiffChanged {
		t.Fatalf("got %+v", d)
	}
	if strings.Contains(d[0].HTML, "<script>") {
		t.Fatalf("text was unescaped: %s", d[0].HTML)
	}
}

func TestEraFieldChanges(t *testing.T) {
	usual := map[string]any{"ruler": "Glassguild", "pop": float64(18000), "status": "Thriving"}
	tests := []struct {
		name string
		full map[string]any
		want map[string]any
	}{
		{"nothing changed", map[string]any{"ruler": "Glassguild", "pop": 18000, "status": "Thriving"}, map[string]any{}},
		{"one changed", map[string]any{"ruler": "Council", "pop": 18000, "status": "Thriving"}, map[string]any{"ruler": "Council"}},
		{"cleared is stored as null", map[string]any{"ruler": "Glassguild", "pop": 18000}, map[string]any{"status": nil}},
		{"empty string counts as cleared", map[string]any{"ruler": "Glassguild", "pop": 18000, "status": ""}, map[string]any{"status": ""}},
		{"new key", map[string]any{"ruler": "Glassguild", "pop": 18000, "status": "Thriving", "motto": "Ash"}, map[string]any{"motto": "Ash"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eraFieldChanges(usual, tt.full)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				gv, ok := got[k]
				if !ok || !sameFieldValue(gv, v) || (v == nil) != (gv == nil) {
					t.Errorf("%s: got %v, want %v", k, gv, v)
				}
			}
		})
	}
	merged := mergeEraFields(usual, map[string]any{"ruler": "Council", "status": nil})
	if merged["ruler"] != "Council" || merged["pop"] != float64(18000) {
		t.Errorf("merge lost a value: %v", merged)
	}
	if _, ok := merged["status"]; ok {
		t.Errorf("a null change should clear the field: %v", merged)
	}
}
