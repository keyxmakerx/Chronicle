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
	"strings"
	"time"
)

// maxFingerprintBytes caps the file read in fingerprintFiles. Widget/manifest
// files are a few KB; the cap stops a hostile manifest that points a widget path
// at a huge file from OOMing the health endpoint (it's reported as too-large
// rather than hashed).
const maxFingerprintBytes = 8 << 20 // 8 MiB

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
	cleanDir := filepath.Clean(dir)
	for _, rel := range relPaths {
		fp := FileFingerprint{Path: rel}
		if dir != "" && rel != "" {
			full := filepath.Clean(filepath.Join(cleanDir, rel))
			// Clamp to the system dir. A hostile manifest could declare a widget
			// path like "../../../etc/passwd"; never stat/read outside dir. (Mirrors
			// the WidgetScriptAPI traversal guard.) Out-of-bounds → Exists=false.
			inDir := full == cleanDir || strings.HasPrefix(full, cleanDir+string(os.PathSeparator))
			if inDir {
				if info, err := os.Stat(full); err == nil && !info.IsDir() {
					fp.Exists = true
					fp.Size = info.Size()
					fp.ModTime = info.ModTime().UTC().Format(time.RFC3339)
					if info.Size() <= maxFingerprintBytes {
						if data, err := os.ReadFile(full); err == nil {
							sum := sha256.Sum256(data)
							fp.SHA256 = hex.EncodeToString(sum[:])[:16]
						}
					} else {
						fp.SHA256 = "too-large"
					}
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
// stable output. The loader maps are snapshotted under the read lock; the
// per-file disk I/O then runs UNLOCKED so a slow stat/read can't block package
// installs (which take the write lock). Manifests are immutable after load, so
// reading their widget lists outside the lock is safe.
func (l *SystemLoader) Health() []SystemHealth {
	type snap struct {
		id       string
		manifest *SystemManifest
		source   string
		dir      string
	}
	l.mu.RLock()
	snaps := make([]snap, 0, len(l.modules))
	for id, ls := range l.modules {
		if ls == nil || ls.manifest == nil {
			continue
		}
		snaps = append(snaps, snap{id: id, manifest: ls.manifest, source: ls.source, dir: ls.dir})
	}
	l.mu.RUnlock()

	out := make([]SystemHealth, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, SystemHealth{
			ID:      s.id,
			Name:    s.manifest.Name,
			Version: s.manifest.Version,
			Source:  s.source,
			Dir:     s.dir,
			Files:   fingerprintFiles(s.dir, healthFilePaths(s.manifest)),
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
