// Package systems — health.go exposes a read-only "deployment health" view of
// every LOADED system: the version + on-disk directory actually being served,
// plus a content fingerprint (size + sha256 + mtime) of each widget/manifest
// file. It answers the operator question "is the server serving the file I
// think it is?" — the failure mode where Admin▸Packages reports one version but
// a stale copy is what actually renders — without shell/SSH access to the host.
//
// Read-only by construction: it only stats and hashes files the loader already
// serves. Surface it behind an admin-gated route.
package systems

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// FileFingerprint identifies a single served file by size + content hash, so two
// installs can be compared without reading the whole file. A missing file
// (Exists=false) is itself diagnostic (e.g. a botched extraction).
type FileFingerprint struct {
	Path    string `json:"path"`             // path relative to the system dir
	Exists  bool   `json:"exists"`           // false → the loader's dir lacks this file
	Size    int64  `json:"size,omitempty"`   // bytes
	SHA256  string `json:"sha256,omitempty"` // first 16 hex chars of the content hash
	ModTime string `json:"mtime,omitempty"`  // file mtime, RFC3339 UTC
}

// SystemHealth is the served reality for one loaded system: the version + dir the
// loader resolves (what WidgetScriptAPI reads from), its source, and fingerprints
// of its served files. If this Version disagrees with Admin▸Packages' installed
// version, the loader never picked up the install (a stale in-memory registry);
// if Version matches but a file's hash is the old one, the extraction is wrong.
type SystemHealth struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Version string            `json:"loaded_version"`
	Source  string            `json:"source"` // "bundled" | "package"
	Dir     string            `json:"dir"`
	Files   []FileFingerprint `json:"files"`
}

// fingerprintFiles stats + hashes each relative path under dir. Pure I/O, no
// globals — unit-tested directly. Never throws: an unreadable/missing file
// yields Exists=false rather than failing the whole report.
func fingerprintFiles(dir string, relPaths []string) []FileFingerprint {
	out := make([]FileFingerprint, 0, len(relPaths))
	for _, rel := range relPaths {
		fp := FileFingerprint{Path: rel}
		if dir != "" && rel != "" {
			full := filepath.Join(dir, rel)
			if info, err := os.Stat(full); err == nil && !info.IsDir() {
				fp.Exists = true
				fp.Size = info.Size()
				fp.ModTime = info.ModTime().UTC().Format(time.RFC3339)
				if data, err := os.ReadFile(full); err == nil {
					sum := sha256.Sum256(data)
					fp.SHA256 = hex.EncodeToString(sum[:])[:16]
				}
			}
		}
		out = append(out, fp)
	}
	return out
}

// healthFilePaths collects the relative paths worth fingerprinting for a system:
// its manifest plus every declared widget script and text-renderer file. These
// are exactly the files the host serves off disk, so a stale copy shows up here.
func healthFilePaths(m *SystemManifest) []string {
	paths := []string{"manifest.json"}
	seen := map[string]bool{"manifest.json": true}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		paths = append(paths, p)
	}
	for i := range m.Widgets {
		add(m.Widgets[i].ScriptFile)
		add(m.Widgets[i].File)
	}
	for i := range m.TextRenderers {
		add(m.TextRenderers[i].File)
	}
	return paths
}

// Health returns the served reality for every loaded system, sorted by ID for
// stable output.
func (l *SystemLoader) Health() []SystemHealth {
	out := make([]SystemHealth, 0, len(l.modules))
	for id, ls := range l.modules {
		if ls == nil || ls.manifest == nil {
			continue
		}
		out = append(out, SystemHealth{
			ID:      id,
			Name:    ls.manifest.Name,
			Version: ls.manifest.Version,
			Source:  ls.source,
			Dir:     ls.dir,
			Files:   fingerprintFiles(ls.dir, healthFilePaths(ls.manifest)),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// LoadedHealth is the package-level accessor over the global loader. Returns nil
// before Init().
func LoadedHealth() []SystemHealth {
	if globalLoader == nil {
		return nil
	}
	return globalLoader.Health()
}
