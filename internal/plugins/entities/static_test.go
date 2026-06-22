package entities

import "testing"

// TestStaticAssetsFS_ContainsCharactersJS pins the embed contract: the
// Characters page enhancement script must ship in the binary so Echo can serve
// it at /static/plugins/entities/js/characters.js.
func TestStaticAssetsFS_ContainsCharactersJS(t *testing.T) {
	f, err := StaticAssetsFS.Open("static/js/characters.js")
	if err != nil {
		t.Fatalf("expected embedded static/js/characters.js: %v", err)
	}
	_ = f.Close()
}
