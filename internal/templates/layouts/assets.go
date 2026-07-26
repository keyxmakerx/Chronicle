// assets.go — cache-busted static asset URLs (C-CAL-MOBILE-VIEWS-FIX, absorbing
// the long-planned C-ASSET-VERSIONING).
//
// WHY this exists. Every static asset shipped as a bare, unversioned path
// (`/static/css/app.css`, `/static/js/*.js`, plugin assets under
// `/static/plugins/<slug>/...`). Echo's `e.Static` serves them through
// `http.ServeContent`, which emits Last-Modified but NO Cache-Control, so
// browsers fall back to HEURISTIC freshness and can hold a build-old copy for
// hours without ever revalidating. Any deploy whose CSS gains a NEW utility
// class then half-lands: the fresh HTML references a class the cached
// stylesheet has never heard of.
//
// That is not hypothetical. C-CAL-MOBILE-VIEWS-FIX Step 0 reproduced it: the
// calendar's phone/desktop pill reduction introduced `md:contents`, a utility
// Tailwind had never emitted before that commit. Rendering post-deploy HTML
// against a pre-deploy app.css drops the Week/Day/Timeline pills from the
// DESKTOP calendar entirely — `hidden md:contents` degrades to plain `hidden`
// when `.md\:contents` is missing from the stylesheet. Same class of failure as
// the "calendar looks old after deploy" reports.
//
// THE FIX. Every asset URL emitted by a template routes through AssetURL, which
// appends `?v=<digest>`:
//
//   - `<digest>` is the first 10 hex chars of the file's SHA-256 when the asset
//     resolves on disk (or through a registered plugin FS). Per-FILE, so a
//     deploy only busts the files that actually changed.
//   - Assets that can't be resolved (an embed FS nobody registered, a path
//     typo) fall back to a per-BUILD token. Coarser, but it still busts on
//     every deploy, so an unresolvable asset degrades to "correct but less
//     efficient" rather than "silently stale".
//
// Digests are computed once per path and cached for the process lifetime;
// static assets do not change under a running binary.
//
// The other half of the fix is middleware.StaticCache (internal/middleware/
// static_cache.go), which turns the versioned URLs into a real caching policy:
// long-lived immutable caching for `?v=`-carrying requests, forced revalidation
// otherwise.
package layouts

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
)

// StaticURLPrefix is the URL prefix every Chronicle static asset lives under.
// Exported so the contract test and the caching middleware agree on one value.
const StaticURLPrefix = "/static/"

// assetResolvers holds the filesystems AssetURL will hash against, in
// registration order. The on-disk `static/` root is seeded by default (it
// mirrors `e.Static("/static", "static")` in internal/app/app.go); plugins that
// serve from an embed FS register theirs via RegisterAssetFS.
var (
	assetMu        sync.RWMutex
	assetResolvers = []assetResolver{{prefix: StaticURLPrefix, fsys: os.DirFS("static")}}
	assetDigests   sync.Map // url path -> digest string
)

// assetResolver maps a URL prefix onto a filesystem rooted at that prefix.
type assetResolver struct {
	prefix string
	fsys   fs.FS
}

// RegisterAssetFS teaches AssetURL to hash assets served from an embedded
// filesystem (plugin static mounts). urlPrefix is the full URL prefix the FS is
// mounted at, e.g. "/static/plugins/calendar/"; fsys is rooted at that prefix.
//
// Registration is optional: an unregistered mount just falls through to the
// per-build token, which still busts on deploy. Later registrations win over
// earlier ones for overlapping prefixes, and the on-disk root stays last so a
// specific plugin FS is always consulted first.
func RegisterAssetFS(urlPrefix string, fsys fs.FS) {
	if fsys == nil || !strings.HasPrefix(urlPrefix, StaticURLPrefix) {
		return
	}
	if !strings.HasSuffix(urlPrefix, "/") {
		urlPrefix += "/"
	}
	assetMu.Lock()
	defer assetMu.Unlock()
	// Prepend: most-specific-registered wins, on-disk root remains the tail.
	assetResolvers = append([]assetResolver{{prefix: urlPrefix, fsys: fsys}}, assetResolvers...)
	// A newly-registered FS can change what a path resolves to, so drop any
	// digests already cached from the fallback path.
	assetDigests.Range(func(k, _ any) bool {
		if key, ok := k.(string); ok && strings.HasPrefix(key, urlPrefix) {
			assetDigests.Delete(k)
		}
		return true
	})
}

// AssetURL returns urlPath with a cache-busting `?v=<digest>` appended.
//
// Non-"/static/" inputs (external CDNs, data: URIs, anything already carrying a
// query string) are returned untouched — versioning a URL we don't serve would
// at best be noise and at worst break a third-party request.
func AssetURL(urlPath string) string {
	if !strings.HasPrefix(urlPath, StaticURLPrefix) || strings.ContainsAny(urlPath, "?#") {
		return urlPath
	}
	return urlPath + "?v=" + assetDigest(urlPath)
}

// assetDigest resolves (and memoizes) one asset's version token.
func assetDigest(urlPath string) string {
	if cached, ok := assetDigests.Load(urlPath); ok {
		return cached.(string)
	}
	digest := hashAsset(urlPath)
	if digest == "" {
		digest = buildToken()
	}
	assetDigests.Store(urlPath, digest)
	return digest
}

// hashAsset walks the registered resolvers for the first one whose prefix
// matches and whose filesystem actually holds the file. Returns "" when no
// resolver can produce the bytes — the caller falls back to the build token.
func hashAsset(urlPath string) string {
	assetMu.RLock()
	resolvers := assetResolvers
	assetMu.RUnlock()

	for _, r := range resolvers {
		if !strings.HasPrefix(urlPath, r.prefix) {
			continue
		}
		// fs.FS paths are slash-separated and never rooted or dot-prefixed.
		rel := path.Clean(strings.TrimPrefix(urlPath, r.prefix))
		if rel == "." || rel == "/" || strings.HasPrefix(rel, "..") {
			continue
		}
		f, err := r.fsys.Open(rel)
		if err != nil {
			continue
		}
		sum := sha256.New()
		_, err = io.Copy(sum, f)
		_ = f.Close()
		if err != nil {
			continue
		}
		return hex.EncodeToString(sum.Sum(nil))[:10]
	}
	return ""
}

// buildToken is the per-BUILD fallback version, derived from the running
// executable's size + modification time. Deliberately NOT process-start time:
// a plain restart of the same binary should not invalidate every client's
// cache, but a deploy (new binary, new mtime) must. Falls back to a constant
// only when the executable can't be stat'd, which in practice means a test
// harness rather than a deployment.
var buildToken = sync.OnceValue(func() string {
	exe, err := os.Executable()
	if err != nil {
		return "dev"
	}
	info, err := os.Stat(exe)
	if err != nil {
		return "dev"
	}
	sum := sha256.Sum256([]byte(strconv.FormatInt(info.Size(), 10) + "-" + strconv.FormatInt(info.ModTime().UnixNano(), 10)))
	return hex.EncodeToString(sum[:])[:10]
})
