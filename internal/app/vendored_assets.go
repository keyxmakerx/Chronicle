// vendored_assets.go locates the files an external client copies unchanged
// in the filesystems Chronicle serves them from, so the sync API can report
// hashes of what this deploy actually runs.

package app

import (
	"io/fs"
	"log/slog"
	"os"
	"path"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

// staticDir is the on-disk directory behind /static (see New in app.go).
const staticDir = "static"

// vendoredAssetFiles maps repo paths onto served filesystems: static/... is
// the on-disk static directory, internal/plugins/<slug>/static/... is the
// embed that plugin mounted. A path no mount serves is left out and logged,
// so the report never names a file this server does not serve.
func vendoredAssetFiles(paths []string, diskStatic fs.FS, regs []PluginRegistration) []syncapi.VendoredAssetFile {
	pluginFS := make(map[string]fs.FS, len(regs))
	for _, r := range regs {
		if r.StaticFS != nil {
			pluginFS[r.Slug] = r.StaticFS
		}
	}
	out := make([]syncapi.VendoredAssetFile, 0, len(paths))
	for _, p := range paths {
		f := syncapi.VendoredAssetFile{Name: path.Base(p)}
		if rest, ok := strings.CutPrefix(p, staticDir+"/"); ok {
			f.FS, f.Path = diskStatic, rest
		} else if rest, ok := strings.CutPrefix(p, "internal/plugins/"); ok {
			slug, inner, _ := strings.Cut(rest, "/")
			if sub, ok := strings.CutPrefix(inner, "static/"); ok {
				f.FS, f.Path = pluginFS[slug], sub
			}
		}
		if f.FS == nil {
			slog.Warn("vendored asset is not served by any static mount", slog.String("path", p))
			continue
		}
		out = append(out, f)
	}
	return out
}

// hashVendoredAssets hashes the served bytes of each path and logs any that
// could not be read, which the sync API then leaves out.
func (a *App) hashVendoredAssets(paths []string) map[string]string {
	files := vendoredAssetFiles(paths, os.DirFS(staticDir), a.registeredPlugins)
	hashes := syncapi.HashVendoredAssets(files)
	for _, f := range files {
		if _, ok := hashes[f.Name]; !ok {
			slog.Warn("vendored asset could not be read", slog.String("name", f.Name))
		}
	}
	return hashes
}
