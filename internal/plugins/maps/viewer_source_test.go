package maps

import (
	"os"
	"testing"
)

// viewerScript returns the map viewer module's source. Several tests pin
// behaviour of the viewer (what a pin edit sends, how the zoom control is
// configured); that behaviour lives in the static module rather than inline in
// the templ, so they read it from here.
func viewerScript(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("../../../static/js/widgets/map_viewer.js")
	if err != nil {
		t.Fatalf("read map_viewer.js: %v", err)
	}
	return string(src)
}

// templSource returns maps.templ for tests that pin what the template's own
// client-side code does.
func templSource(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("maps.templ")
	if err != nil {
		t.Fatalf("read maps.templ: %v", err)
	}
	return string(src)
}
