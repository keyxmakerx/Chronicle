package foundry_vtt

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// TestVendoredManifestMatchesFiles is the go-test half of
// tools/check-foundry-vendored.sh: a change to a file the module copies
// fails here too, so it is caught by whoever runs go test, not only CI.
func TestVendoredManifestMatchesFiles(t *testing.T) {
	assets := VendoredAssets()
	if len(assets) == 0 {
		t.Fatal("manifest lists no files")
	}
	root := filepath.Join("..", "..", "..")
	for _, a := range assets {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(a.Path)))
		if err != nil {
			t.Errorf("%s: %v (the Foundry module keeps a copy of this file)", a.Path, err)
			continue
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:])[:12]; got != a.Hash {
			t.Errorf("%s changed: hash %s, manifest says %s. The Foundry module keeps a copy of this file; update its hash in vendored-assets.txt and open a module refresh", a.Path, got, a.Hash)
		}
	}
}

func TestParseVendoredAssets(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		want    int
		wantErr bool
	}{
		{"comments and blanks skipped", "# head\n\n0123456789ab  static/js/a.js  sky\n", 1, false},
		{"two entries", "0123456789ab  static/a.js  sky\nabcdefabcdef  x/b.css  shop\n", 2, false},
		{"short hash", "0123  static/a.js  sky\n", 0, true},
		{"missing engine", "0123456789ab  static/a.js\n", 0, true},
		{"upper-case hash", "0123456789AB  static/a.js  sky\n", 0, true},
		{"duplicate copy name", "0123456789ab  a/x.js  sky\n0123456789ab  b/x.js  shop\n", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVendoredAssets(tt.text)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != tt.want {
				t.Fatalf("got %d entries, want %d", len(got), tt.want)
			}
		})
	}
}

func TestVendoredAssetCopyName(t *testing.T) {
	a := VendoredAsset{Path: "internal/plugins/crafting/static/css/spoils.css"}
	if got := a.CopyName(); got != "spoils.css" {
		t.Fatalf("CopyName = %q", got)
	}
}
