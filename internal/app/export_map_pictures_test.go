package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestImportTokenImage: a token keeps a picture that names no media file,
// and one naming a media file only when the import restored that file.
func TestImportTokenImage(t *testing.T) {
	const restored = "bbbbbbbb-0000-0000-0000-000000000001"
	const foreign = "cccccccc-0000-0000-0000-000000000002"
	idMap := campaigns.NewIDMap("c")
	idMap.MediaIDs["aaaaaaaa-0000-0000-0000-000000000001"] = restored
	str := func(s string) *string { return &s }

	cases := []struct {
		name   string
		in     *string
		wantOK bool
		kept   bool
	}{
		{"none", nil, true, false},
		{"bundled icon", str("wolf.png"), true, true},
		{"inline data image", str("data:image/png;base64,AAAA"), true, true},
		{"restored id", str(restored), true, true},
		{"restored media address", str("/media/" + restored), true, true},
		{"restored stored path", str("2026/03/" + restored + ".png"), true, true},
		{"another campaign's id", str(foreign), false, false},
		{"another campaign's media address", str("/media/" + foreign), false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := importTokenImage(idMap, tc.in)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if kept := got != nil; kept != tc.kept || (kept && *got != *tc.in) {
				t.Errorf("got %v, want kept=%v", got, tc.kept)
			}
		})
	}
}
