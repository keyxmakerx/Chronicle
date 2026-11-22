package cardborders

import (
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestLookFrom(t *testing.T) {
	tests := []struct {
		name  string
		props map[string]any
		want  Look
		ok    bool
	}{
		{"valid border and colour", map[string]any{"card_border": "fire", "card_colour": "#112233"}, Look{"fire", "#112233"}, true},
		{"upper-case hex is lower-cased", map[string]any{"card_border": "fire", "card_colour": "#AABBCC"}, Look{"fire", "#aabbcc"}, true},
		{"missing colour takes the default", map[string]any{"card_border": "scales"}, Look{"scales", "#2fb5a4"}, true},
		{"short colour takes the default", map[string]any{"card_border": "ice", "card_colour": "#fff"}, Look{"ice", "#8fd8ff"}, true},
		{"no hash takes the default", map[string]any{"card_border": "ice", "card_colour": "8fd8ff"}, Look{"ice", "#8fd8ff"}, true},
		{"non-hex digits take the default", map[string]any{"card_border": "vine", "card_colour": "#gggggg"}, Look{"vine", "#6fae4f"}, true},
		{"colour of the wrong type takes the default", map[string]any{"card_border": "vine", "card_colour": 12345.0}, Look{"vine", "#6fae4f"}, true},
		{"unknown key", map[string]any{"card_border": "lava", "card_colour": "#112233"}, Look{}, false},
		{"key is case sensitive", map[string]any{"card_border": "Fire"}, Look{}, false},
		{"border of the wrong type", map[string]any{"card_border": 3.0}, Look{}, false},
		{"border a list", map[string]any{"card_border": []any{"fire"}}, Look{}, false},
		{"colour alone is no look", map[string]any{"card_colour": "#112233"}, Look{}, false},
		{"no properties", nil, Look{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := LookFrom(tt.props)
			if ok != tt.ok || got != tt.want {
				t.Errorf("LookFrom = %+v, %v; want %+v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestLibraryShape(t *testing.T) {
	if n := len(All()); n != 16 {
		t.Fatalf("library has %d borders, want 16", n)
	}
	seen := map[string]bool{}
	for _, b := range All() {
		if b.Key == "" || b.Name == "" || seen[b.Key] {
			t.Errorf("border %+v needs a unique key and a name", b)
		}
		seen[b.Key] = true
		if !ValidColour(b.Colour) || b.Colour != strings.ToLower(b.Colour) {
			t.Errorf("%s: default colour %q is not lower-case #rrggbb", b.Key, b.Colour)
		}
		if got, ok := Get(b.Key); !ok || got.Key != b.Key {
			t.Errorf("Get(%q) = %v, %v", b.Key, got.Key, ok)
		}
		if (b.Mask != "" || b.CornerMask != "") && (b.Peak == nil || b.Gain == nil) {
			t.Errorf("%s has a mask but no peak and gain", b.Key)
		}
		if b.Glow != nil && (b.Mask == "" && b.CornerMask == "") {
			t.Errorf("%s glows but has no mask, so nothing takes the glow", b.Key)
		}
		if b.Glow != nil && *b.Glow <= 0 {
			t.Errorf("%s: glow %v must be positive", b.Key, *b.Glow)
		}
	}
	if !reflect.DeepEqual(Keys()[:3], []string{"carved-stone", "earth", "fire"}) {
		t.Errorf("order changed: %v", Keys())
	}
	if _, ok := Get("nope"); ok {
		t.Error("Get found a border that is not there")
	}
	all := All()
	all[0].Key = "changed"
	if All()[0].Key == "changed" {
		t.Error("All hands out the library itself")
	}
}

// Every picture the library names is a real file in the plugin's static
// folder, which is what the page embeds and serves.
func TestPicturesExist(t *testing.T) {
	dir := os.DirFS("../static/img/borders")
	for _, f := range Files() {
		if _, err := fs.Stat(dir, f); err != nil {
			t.Errorf("library names %s, which is not in static/img/borders: %v", f, err)
		}
	}
}

// Files is what the page embeds and serves, so a picture it drops is a 404.
func TestFilesCoversEveryPictureField(t *testing.T) {
	got := map[string]bool{}
	for _, f := range Files() {
		got[f] = true
	}
	for _, b := range All() {
		for field, f := range map[string]string{"Img": b.Img, "Mask": b.Mask, "Over": b.Over, "Corner": b.Corner, "CornerMask": b.CornerMask, "Ground": b.Ground} {
			if f != "" && !got[f] {
				t.Errorf("%s.%s names %s, which Files() leaves out", b.Key, field, f)
			}
		}
	}
	if m, _ := Get("mechanical"); m.Ground == "" || !got[m.Ground] {
		t.Errorf("mechanical's ground %q must be in Files()", m.Ground)
	}
}

func TestLibraryJSON(t *testing.T) {
	got := LibraryJSON(func(p string) string { return p + "?v=x" })
	for _, want := range []string{
		`"great-rune":{"img":"/static/plugins/entities/img/borders/great-rune.webp?v=x","mask":"/static/plugins/entities/img/borders/great-rune-mask.webp?v=x","peak":0,"gain":0.85}`,
		`"mechanical":{"img":"/static/plugins/entities/img/borders/mechanical.webp?v=x","mask":"/static/plugins/entities/img/borders/mechanical-mask.webp?v=x","corner":"/static/plugins/entities/img/borders/mechanical-corners.webp?v=x","cornerMask":"/static/plugins/entities/img/borders/mechanical-corners-mask.webp?v=x","ground":"/static/plugins/entities/img/borders/mechanical-steam.webp?v=x","peak":0,"gain":1,"glow":1.4}`,
		`"scales":{"img":"/static/plugins/entities/img/borders/scales.webp?v=x","over":"/static/plugins/entities/img/borders/scales-boss.webp?v=x","dye":true}`,
		`"carved-stone":{"img":"/static/plugins/entities/img/borders/earth.webp?v=x","corner":`,
		`"cornerMask":"/static/plugins/entities/img/borders/carved-stone-corner-mask.webp?v=x"`,
		`"earth":{"img":"/static/plugins/entities/img/borders/earth.webp?v=x"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("library JSON lacks %s\n%s", want, got)
		}
	}
	if strings.Contains(got, "Carved stone") || strings.Contains(got, "#e0a030") {
		t.Error("names and default colours stay on the server")
	}
}
