package maps

import (
	"os"
	"strings"
	"testing"
)

// The hex-cover controls are client code, so their contract is pinned from the
// source: the wording the approved design fixes, and the two rules that keep a
// layer change safe (the layer endpoint carries only the anchor, and painted
// hexes are never re-laid without asking).
func TestHexCoverControls_Source(t *testing.T) {
	viewer := viewerScript(t)
	pictures, err := os.ReadFile("static/js/map_pictures.js")
	if err != nil {
		t.Fatal(err)
	}
	hexesSrc, err := os.ReadFile("../../../static/js/map_hexes.js")
	if err != nil {
		t.Fatal(err)
	}
	hexes := string(hexesSrc)
	sheet, err := os.ReadFile("map_settings_sheet.templ")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, src, want string
	}{
		{"flyout, whole map", viewer, "item('Whole map', 'Hexes cover everything'"},
		{"flyout, one picture", viewer, "'Hexes cover just this picture'"},
		{"flyout, helper text with pictures", viewer, "You can also click any picture and choose Turn into a hex map."},
		{"flyout, helper text without", viewer, "Add a picture first to turn just that picture into a hex map."},
		{"take-off confirm", viewer, "Take the hexes off this map? Painted hexes are kept, so making it a hex map again brings them back."},
		{"hexes panel, take off", hexes, ">Take the hexes off</button>"},
		{"re-lay confirm", viewer, "Painted hexes stay where they are in the grid, so they may land on different spots. Continue?"},
		{"chips, one picture", viewer, "Just “"},
		{"picture bar, way in", string(pictures), "Turn into a hex map"},
		{"picture bar, what it is", string(pictures), "This picture is a hex map. The hexes move and resize with it."},
		{"picture bar, open", string(pictures), "Open hexes"},
		{"picture bar, take off", string(pictures), "Take the hexes off"},
		{"settings, cover row", string(sheet), "Hexes cover"},
		{"anchor sent alone", viewer, "{ anchor_drawing_id: id }"},
		{"anchor cleared with null", viewer, "{ anchor_drawing_id: null }"},
	}
	for _, tc := range tests {
		if !strings.Contains(tc.src, tc.want) {
			t.Errorf("%s: missing %q", tc.name, tc.want)
		}
	}
	// Every path that moves the hexes asks first; counting the confirm keeps a
	// new path from skipping it unnoticed.
	if n := strings.Count(viewer, "confirm(RELAY_CONFIRM)"); n != 2 {
		t.Errorf("RELAY_CONFIRM is asked in %d places, want 2 (setHexCover and the settings save)", n)
	}
}
