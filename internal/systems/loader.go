package systems

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
)

// SystemLoader discovers and loads system manifests from a directory tree.
// Each subdirectory containing a manifest.json is treated as a system.
// Invalid manifests are logged as warnings but do not prevent startup.
type SystemLoader struct {
	systemsDir      string
	modules         map[string]*loadedSystem
	systemInstances map[string]System
}

// loadedSystem pairs a parsed manifest with its source directory path.
type loadedSystem struct {
	manifest *SystemManifest
	dir      string
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
			continue
		}

		sysDir := filepath.Join(l.systemsDir, entry.Name())
		l.modules[manifest.ID] = &loadedSystem{
			manifest: manifest,
			dir:      sysDir,
		}

		slog.Info("discovered system",
			slog.String("id", manifest.ID),
			slog.String("name", manifest.Name),
			slog.String("version", manifest.Version),
			slog.String("status", string(manifest.Status)),
		)

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
					continue
				}
				l.RegisterSystem(mod)
				slog.Info("instantiated system",
					slog.String("id", manifest.ID),
				)
			} else {
				// No custom factory — try generic system with JSON data.
				mod, err := NewGenericSystem(manifest, dataDir)
				if err != nil {
					slog.Warn("failed to instantiate generic system",
						slog.String("id", manifest.ID),
						slog.String("error", err.Error()),
					)
					continue
				}
				l.RegisterSystem(mod)
				slog.Info("instantiated generic system",
					slog.String("id", manifest.ID),
				)
			}
		}
	}

	return nil
}

// DiscoverDir scans a single directory for a manifest.json file and loads it
// as a system. If a system with the same ID already exists, it is replaced.
// This is used to overlay package-manager-installed systems on top of bundled ones.
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

// loadSingleSystem loads a system from a directory containing manifest.json.
func (l *SystemLoader) loadSingleSystem(sysDir string) error {
	manifestPath := filepath.Join(sysDir, "manifest.json")
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("invalid manifest in %s: %w", sysDir, err)
	}

	l.modules[manifest.ID] = &loadedSystem{
		manifest: manifest,
		dir:      sysDir,
	}

	slog.Info("loaded package system",
		slog.String("id", manifest.ID),
		slog.String("name", manifest.Name),
		slog.String("dir", sysDir),
	)

	// Instantiate if available.
	if manifest.Status == StatusAvailable {
		dataDir := filepath.Join(sysDir, "data")
		if factory, ok := factories[manifest.ID]; ok {
			mod, fErr := factory(manifest, dataDir)
			if fErr != nil {
				return fmt.Errorf("factory failed for %s: %w", manifest.ID, fErr)
			}
			l.RegisterSystem(mod)
		} else {
			mod, gErr := NewGenericSystem(manifest, dataDir)
			if gErr != nil {
				return fmt.Errorf("generic system failed for %s: %w", manifest.ID, gErr)
			}
			l.RegisterSystem(mod)
		}
	}

	return nil
}

// Get returns the manifest for a system by ID, or nil if not found.
func (l *SystemLoader) Get(id string) *SystemManifest {
	if lm, ok := l.modules[id]; ok {
		return lm.manifest
	}
	return nil
}

// All returns all discovered system manifests, sorted alphabetically by name.
func (l *SystemLoader) All() []*SystemManifest {
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
	if lm, ok := l.modules[id]; ok {
		return lm.dir
	}
	return ""
}

// Count returns the number of discovered systems.
func (l *SystemLoader) Count() int {
	return len(l.modules)
}

// RegisterSystem registers a live System instance. Called during
// discovery for systems with status "available" that have data loaded.
func (l *SystemLoader) RegisterSystem(mod System) {
	l.systemInstances[mod.Info().ID] = mod
}

// GetSystem returns the live System instance by ID, or nil if not
// found or not instantiated.
func (l *SystemLoader) GetSystem(id string) System { 
	return l.systemInstances[id]
}

// AllSystems returns all live System instances.
func (l *SystemLoader) AllSystems() []System {
	result := make([]System, 0, len(l.systemInstances))
	for _, m := range l.systemInstances {
		result = append(result, m)
	}
	return result
}
