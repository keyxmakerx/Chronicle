package app

import (
	"testing"
	"testing/fstest"
)

func TestVendoredAssetFiles(t *testing.T) {
	disk := fstest.MapFS{"js/widgets/sky_gl.js": {Data: []byte("sky")}}
	craft := fstest.MapFS{"css/spoils.css": {Data: []byte("spoils")}}
	regs := []PluginRegistration{{Slug: "crafting", StaticFS: craft}, {Slug: "noassets"}}
	tests := []struct {
		name     string
		path     string
		wantName string
		wantPath string
		wantFS   any
	}{
		{"core static file", "static/js/widgets/sky_gl.js", "sky_gl.js", "js/widgets/sky_gl.js", disk},
		{"plugin embed file", "internal/plugins/crafting/static/css/spoils.css", "spoils.css", "css/spoils.css", craft},
		{"plugin with no mount", "internal/plugins/noassets/static/x.js", "", "", nil},
		{"unregistered plugin", "internal/plugins/ghost/static/x.js", "", "", nil},
		{"outside any mount", "internal/plugins/crafting/handler.go", "", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vendoredAssetFiles([]string{tt.path}, disk, regs)
			if tt.wantFS == nil {
				if len(got) != 0 {
					t.Fatalf("got %+v, want it left out", got)
				}
				return
			}
			if len(got) != 1 || got[0].Name != tt.wantName || got[0].Path != tt.wantPath {
				t.Fatalf("got %+v", got)
			}
			if _, ok := got[0].FS.(fstest.MapFS); !ok {
				t.Fatalf("FS not resolved: %T", got[0].FS)
			}
		})
	}
}
