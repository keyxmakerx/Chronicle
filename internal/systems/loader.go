package systems

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// SystemLoader discovers and loads system manifests from a directory tree.
// Each subdirectory containing a manifest.json is treated as a system.
// Invalid manifests are logged as warnings but do not prevent startup.
type SystemLoader struct {
	// mu guards modules + systemInstances. Reads (Get/Dir/All/Health/…) take
	// RLock; the only writers are register + RegisterSystem (Lock). Package
	// installs re-scan the registry at runtime (ScanPackageDir → register),
	// concurrent with HTTP read handlers, so the maps must be synchronized.
	mu              sync.RWMutex
	systemsDir      string
	modules         map[string]*loadedSystem
	systemInstances map[string]System
}

// loadedSystem pairs a parsed manifest with its source directory path.
type loadedSystem struct {
	manifest *SystemManifest
	dir      string
	source   string // "bundled" or "package" — drives the duplicate-resolution tie-break.
}

// preferCandidate reports whether a newly-discovered manifest should replace the
// currently-loaded system with the same ID. Policy (WS-6 — "never silently
// downgrade"): the highest version wins; on an exact-version tie a
// package-installed copy overlays a bundled one (the intended package-manager
// override); anything else keeps what's already loaded. This stops a stale
// duplicate directory (e.g. a leftover "<slug>-1") from shadowing the current
// system purely because it sorts later in the scan.
func (l *SystemLoader) preferCandidate(existing *loadedSystem, candidate *SystemManifest, source string) bool {
	if existing == nil {
		return true
	}
	ev, cv := existing.manifest.Version, candidate.Version
	if versionLess(ev, cv) {
		return true // candidate strictly newer → wins
	}
	if versionLess(cv, ev) {
		return false // candidate older → never clobbers the newer load
	}
	// Equal version: let a package overlay a bundled one; otherwise keep first.
	return source == "package" && existing.source == "bundled"
}

// register records a discovered manifest under its ID, applying the
// duplicate-resolution policy (preferCandidate). It returns true when the
// candidate was accepted (the caller should then instantiate it) and false when
// a preferred copy is already loaded — in which case it logs and records an
// EventSkipped so the ignored duplicate is visible in admin diagnostics, and the
// caller must NOT instantiate (which would re-introduce the last-wins bug via
// systemInstances).
func (l *SystemLoader) register(manifest *SystemManifest, dir, source string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	existing := l.modules[manifest.ID]
	if !l.preferCandidate(existing, manifest, source) {
		slog.Warn("ignoring duplicate system — a preferred copy is already loaded",
			slog.String("id", manifest.ID),
			slog.String("ignored_dir", dir),
			slog.String("ignored_version", manifest.Version),
			slog.String("kept_dir", existing.dir),
			slog.String("kept_version", existing.manifest.Version),
		)
		RecordEvent(LoadEvent{
			SystemID: manifest.ID,
			Name:     manifest.Name,
			Kind:     EventSkipped,
			Source:   source,
			Error: fmt.Sprintf("duplicate ignored: version %q not preferred over loaded %q (%s)",
				manifest.Version, existing.manifest.Version, existing.dir),
			Dir: dir,
		})
		return false
	}
	if existing != nil {
		slog.Info("replacing system with preferred copy",
			slog.String("id", manifest.ID),
			slog.String("old_dir", existing.dir),
			slog.String("old_version", existing.manifest.Version),
			slog.String("new_dir", dir),
			slog.String("new_version", manifest.Version),
			slog.String("source", source),
		)
	}
	l.modules[manifest.ID] = &loadedSystem{manifest: manifest, dir: dir, source: source}
	return true
}

// NewSystemLoader creates a loader that will scan the given directory
// for system subdirectories containing manifest.json files.
func NewSystemLoader(systemsDir string) *SystemLoader {
	return &SystemLoader{
		systemsDir:      systemsDir,
		modules:         make(map[string]*loadedSystem),
		systemInstances: make(map[string]System),
	}
}

// DiscoverAll scans the systems directory for subdirectories containing
// manifest.json files. Each valid manifest is loaded and registered.
// Invalid manifests are logged as warnings but do not cause an error.
// Returns an error only if the systems directory itself cannot be read.
func (l *SystemLoader) DiscoverAll() error {
	entries, err := os.ReadDir(l.systemsDir)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("systems directory not found, skipping discovery",
				slog.String("dir", l.systemsDir),
			)
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(l.systemsDir, entry.Name(), "manifest.json")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}

		manifest, err := LoadManifest(manifestPath)
		if err != nil {
			slog.Warn("skipping invalid system manifest",
				slog.String("dir", entry.Name()),
				slog.String("error", err.Error()),
			)
			RecordEvent(LoadEvent{
				SystemID: entry.Name(),
				Kind:     EventFailed,
				Source:   "bundled",
				Error:    err.Error(),
				Dir:      filepath.Join(l.systemsDir, entry.Name()),
			})
			continue
		}

		sysDir := filepath.Join(l.systemsDir, entry.Name())
		// Resolve duplicates by version (WS-6) — skip a stale/older copy so it
		// can't shadow the current one by scan order.
		if !l.register(manifest, sysDir, "bundled") {
			continue
		}

		RecordEvent(LoadEvent{
			SystemID: manifest.ID,
			Name:     manifest.Name,
			Kind:     EventDiscovered,
			Source:   "bundled",
			Dir:      sysDir,
		})

		// Attempt to instantiate available modules via registered factories.
		// If no factory is registered, fall back to the generic system
		// (manifest + JSON data + generic tooltip renderer). This allows
		// new game systems to work with zero custom Go code.
		if manifest.Status == StatusAvailable {
			dataDir := filepath.Join(sysDir, "data")
			if factory, ok := factories[manifest.ID]; ok {
				mod, err := factory(manifest, dataDir)
				if err != nil {
					slog.Warn("failed to instantiate system",
						slog.String("id", manifest.ID),
						slog.String("error", err.Error()),
					)
					RecordEvent(LoadEvent{
						SystemID: manifest.ID,
						Name:     manifest.Name,
						Kind:     EventFailed,
						Source:   "bundled",
						Error:    "instantiation: " + err.Error(),
						Dir:      sysDir,
					})
					continue
				}
				l.RegisterSystem(mod)
				RecordEvent(LoadEvent{
					SystemID: manifest.ID,
					Name:     manifest.Name,
					Kind:     EventInstantiated,
					Source:   "bundled",
					Dir:      sysDir,
				})
			} else {
				// No custom factory — try generic system with JSON data.
				mod, err := NewGenericSystem(manifest, dataDir)
				if err != nil {
					slog.Warn("failed to instantiate generic system",
						slog.String("id", manifest.ID),
						slog.String("error", err.Error()),
					)
					RecordEvent(LoadEvent{
						SystemID: manifest.ID,
						Name:     manifest.Name,
						Kind:     EventFailed,
						Source:   "bundled",
						Error:    "generic instantiation: " + err.Error(),
						Dir:      sysDir,
					})
					continue
				}
				l.RegisterSystem(mod)
				RecordEvent(LoadEvent{
					SystemID: manifest.ID,
					Name:     manifest.Name,
					Kind:     EventInstantiated,
					Source:   "bundled",
					Dir:      sysDir,
				})
			}
		}
	}

	return nil
}

// DiscoverDir scans a single directory for a manifest.json file and loads it as
// a system. Used to overlay package-manager-installed systems on top of bundled
// ones. Whether an existing same-ID system is replaced is decided by the
// version-aware policy in register/preferCandidate (WS-6): a package copy
// overlays a bundled one at equal version, a newer version always wins, and an
// older/stale duplicate is skipped rather than silently downgrading the load.
func (l *SystemLoader) DiscoverDir(dir string) error {
	manifestPath := filepath.Join(dir, "manifest.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		// No manifest — try scanning subdirectories (the install dir may
		// contain a single repo root with subdirs per system).
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil // Not a valid directory, skip silently.
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			subManifest := filepath.Join(dir, entry.Name(), "manifest.json")
			if _, err := os.Stat(subManifest); err == nil {
				if err := l.loadSingleSystem(filepath.Join(dir, entry.Name())); err != nil {
					slog.Warn("failed to load package system",
						slog.String("dir", entry.Name()),
						slog.String("error", err.Error()),
					)
				}
			}
		}
		return nil
	}

	return l.loadSingleSystem(dir)
}

// forceRegister replaces any loaded copy of manifest.ID unconditionally,
// bypassing the WS-6 preferCandidate version policy. Reserved for the
// explicit-install path: when the admin installs a SPECIFIC version —
// possibly OLDER than what's loaded (a deliberate rollback) — the
// highest-version policy would silently keep the newer copy, making the
// rollback a no-op. An explicit install is operator intent; it wins.
func (l *SystemLoader) forceRegister(manifest *SystemManifest, dir, source string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if existing := l.modules[manifest.ID]; existing != nil && existing.dir != dir {
		slog.Info("force-replacing system (explicit install)",
			slog.String("id", manifest.ID),
			slog.String("old_dir", existing.dir),
			slog.String("old_version", existing.manifest.Version),
			slog.String("new_dir", dir),
			slog.String("new_version", manifest.Version),
		)
	}
	l.modules[manifest.ID] = &loadedSystem{manifest: manifest, dir: dir, source: source}
}

// ForceLoadDir loads the system at dir and replaces any loaded copy of the
// same system ID regardless of version comparison. Called by the app layer
// after a package install so the loader serves exactly what the admin
// installed — including deliberate rollbacks to an older version, which
// the regular rescan ("highest version wins") would silently ignore.
func ForceLoadDir(dir string) error {
	if globalLoader == nil {
		return fmt.Errorf("system loader not initialized")
	}
	return globalLoader.loadSingleSystemOpts(dir, true)
}

// loadSingleSystem loads a system from a directory containing manifest.json,
// applying the WS-6 duplicate-resolution policy.
func (l *SystemLoader) loadSingleSystem(sysDir string) error {
	return l.loadSingleSystemOpts(sysDir, false)
}

// loadSingleSystemOpts is loadSingleSystem with an explicit force switch:
// force=true replaces any loaded same-ID copy unconditionally (explicit
// installs / rollbacks); force=false applies preferCandidate as usual.
func (l *SystemLoader) loadSingleSystemOpts(sysDir string, force bool) error {
	manifestPath := filepath.Join(sysDir, "manifest.json")
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("invalid manifest in %s: %w", sysDir, err)
	}

	// Resolve duplicates by version (WS-6). A package overlay of equal version
	// wins over bundled; an older/duplicate package copy is skipped (not an
	// error — the preferred copy is already loaded). force bypasses the
	// policy entirely (explicit operator intent).
	if force {
		l.forceRegister(manifest, sysDir, "package")
	} else if !l.register(manifest, sysDir, "package") {
		return nil
	}

	slog.Info("loaded package system",
		slog.String("id", manifest.ID),
		slog.String("name", manifest.Name),
		slog.String("dir", sysDir),
	)
	RecordEvent(LoadEvent{
		SystemID: manifest.ID,
		Name:     manifest.Name,
		Kind:     EventDiscovered,
		Source:   "package",
		Dir:      sysDir,
	})

	// Instantiate if available.
	if manifest.Status == StatusAvailable {
		dataDir := filepath.Join(sysDir, "data")
		if factory, ok := factories[manifest.ID]; ok {
			mod, fErr := factory(manifest, dataDir)
			if fErr != nil {
				RecordEvent(LoadEvent{
					SystemID: manifest.ID,
					Name:     manifest.Name,
					Kind:     EventFailed,
					Source:   "package",
					Error:    "factory: " + fErr.Error(),
					Dir:      sysDir,
				})
				return fmt.Errorf("factory failed for %s: %w", manifest.ID, fErr)
			}
			l.RegisterSystem(mod)
		} else {
			mod, gErr := NewGenericSystem(manifest, dataDir)
			if gErr != nil {
				RecordEvent(LoadEvent{
					SystemID: manifest.ID,
					Name:     manifest.Name,
					Kind:     EventFailed,
					Source:   "package",
					Error:    "generic: " + gErr.Error(),
					Dir:      sysDir,
				})
				return fmt.Errorf("generic system failed for %s: %w", manifest.ID, gErr)
			}
			l.RegisterSystem(mod)
		}
		RecordEvent(LoadEvent{
			SystemID: manifest.ID,
			Name:     manifest.Name,
			Kind:     EventInstantiated,
			Source:   "package",
			Dir:      sysDir,
		})
	}

	return nil
}

// Get returns the manifest for a system by ID, or nil if not found.
func (l *SystemLoader) Get(id string) *SystemManifest {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if lm, ok := l.modules[id]; ok {
		return lm.manifest
	}
	return nil
}

// All returns all discovered system manifests, sorted alphabetically by name.
func (l *SystemLoader) All() []*SystemManifest {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make([]*SystemManifest, 0, len(l.modules))
	for _, lm := range l.modules {
		result = append(result, lm.manifest)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Dir returns the absolute directory path for a system by ID, or empty string.
func (l *SystemLoader) Dir(id string) string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if lm, ok := l.modules[id]; ok {
		return lm.dir
	}
	return ""
}

// Count returns the number of discovered systems.
func (l *SystemLoader) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.modules)
}

// RegisterSystem registers a live System instance. Called during
// discovery for systems with status "available" that have data loaded.
func (l *SystemLoader) RegisterSystem(mod System) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.systemInstances[mod.Info().ID] = mod
}

// GetSystem returns the live System instance by ID, or nil if not
// found or not instantiated.
func (l *SystemLoader) GetSystem(id string) System {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.systemInstances[id]
}

// AllSystems returns all live System instances.
func (l *SystemLoader) AllSystems() []System {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make([]System, 0, len(l.systemInstances))
	for _, m := range l.systemInstances {
		result = append(result, m)
	}
	return result
}
