package syncapi

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"sync/atomic"

	"github.com/labstack/echo/v5"
)

// VendoredAssetFile is one file an external client keeps an unchanged copy
// of, located in the filesystem Chronicle actually serves it from. Hashing
// those bytes, rather than quoting the repo's manifest, is what lets a
// deploy whose files differ from the repo report the truth.
type VendoredAssetFile struct {
	Name string // the client's copy name (the file's basename)
	FS   fs.FS  // the served filesystem: the static dir or a plugin's embed
	Path string // the file's path inside FS
}

// vendoredHashLen is how much of the sha256 hex is reported: enough to tell
// two versions of a file apart, and the length the repo manifest uses.
const vendoredHashLen = 12

// HashVendoredAssets returns copy name → first 12 hex chars of the sha256 of
// each file's bytes. A file that cannot be read is left out: an absent entry
// tells the client "unknown", where a made-up hash would claim a mismatch.
func HashVendoredAssets(files []VendoredAssetFile) map[string]string {
	out := make(map[string]string, len(files))
	for _, f := range files {
		if f.FS == nil {
			continue
		}
		b, err := fs.ReadFile(f.FS, f.Path)
		if err != nil {
			continue
		}
		sum := sha256.Sum256(b)
		out[f.Name] = hex.EncodeToString(sum[:])[:vendoredHashLen]
	}
	return out
}

// VendoredAssetsHandler answers which versions of the vendored page scripts
// this server runs, so the Foundry module can tell when its copies are
// stale. Only hashes are served, never code, and only to a key for the
// campaign: exact file hashes fingerprint the build, so they stay off the
// public version endpoint.
type VendoredAssetsHandler struct {
	assets atomic.Pointer[map[string]string]
}

// NewVendoredAssetsHandler returns a handler that answers an empty set until
// SetAssets runs. The hashes are filled once every plugin's static files are
// mounted, which is after the sync API's routes are registered.
func NewVendoredAssetsHandler() *VendoredAssetsHandler {
	h := &VendoredAssetsHandler{}
	h.SetAssets(nil)
	return h
}

// SetAssets replaces the reported hashes. The map is copied so a caller
// cannot change what is served after the fact.
func (h *VendoredAssetsHandler) SetAssets(assets map[string]string) {
	cp := make(map[string]string, len(assets))
	for k, v := range assets {
		cp[k] = v
	}
	h.assets.Store(&cp)
}

// vendoredAssetsResponse is the wire shape; a struct keeps "assets" an
// object even when it is empty.
type vendoredAssetsResponse struct {
	Assets map[string]string `json:"assets"`
}

// GetVendoredAssets handles GET /api/v1/campaigns/:id/vendored-assets.
func (h *VendoredAssetsHandler) GetVendoredAssets(c *echo.Context) error {
	return c.JSON(http.StatusOK, vendoredAssetsResponse{Assets: *h.assets.Load()})
}

// WithVendoredAssets mounts the vendored-assets route on the sync API.
func WithVendoredAssets(h *VendoredAssetsHandler) func(*APIHandler) {
	return func(api *APIHandler) { api.vendoredAssets = h }
}
