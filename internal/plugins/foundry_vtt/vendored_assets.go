package foundry_vtt

import (
	_ "embed"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// vendoredManifest lists the Chronicle files the Foundry module keeps
// unchanged copies of. Embedded so the list ships in the binary and the
// sync API can say which files to report without reading the repo.
//
//go:embed vendored-assets.txt
var vendoredManifest string

// VendoredAsset is one manifest line: a file the module copies byte for byte.
// Hash is what the file held when the manifest was last updated; it pins the
// repo, not a deploy, so anything reporting a live server hashes the bytes it
// serves instead of quoting this.
type VendoredAsset struct {
	Hash   string // first 12 hex chars of the sha256 of the file's bytes
	Path   string // repo-relative path, e.g. static/js/widgets/sky_gl.js
	Engine string // which module engine the copy belongs to
}

// CopyName is the file's name in the module's vendor folder.
func (a VendoredAsset) CopyName() string { return path.Base(a.Path) }

var vendoredHashRe = regexp.MustCompile(`^[0-9a-f]{12}$`)

// ParseVendoredAssets reads the manifest format. A malformed line is an
// error, not a skip: a silently dropped line is a file whose drift nobody
// would see.
func ParseVendoredAssets(text string) ([]VendoredAsset, error) {
	var out []VendoredAsset
	seen := map[string]string{}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 3 || !vendoredHashRe.MatchString(f[0]) {
			return nil, fmt.Errorf("vendored-assets.txt line %d: want <12-hex>  <path>  <engine>", i+1)
		}
		a := VendoredAsset{Hash: f[0], Path: f[1], Engine: f[2]}
		// The module names copies by basename, so two files sharing one
		// would overwrite each other there.
		if prev, dup := seen[a.CopyName()]; dup {
			return nil, fmt.Errorf("vendored-assets.txt line %d: copy name %q also used by %s", i+1, a.CopyName(), prev)
		}
		seen[a.CopyName()] = a.Path
		out = append(out, a)
	}
	return out, nil
}

// VendoredAssets returns the embedded manifest's entries. The manifest is
// checked by this package's tests, so a parse failure here means a broken
// build and panics at startup rather than reporting a short list.
func VendoredAssets() []VendoredAsset {
	a, err := ParseVendoredAssets(vendoredManifest)
	if err != nil {
		panic(err)
	}
	return a
}
