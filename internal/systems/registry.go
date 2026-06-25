// Package modules defines the system registry for Chronicle.
// Modules are game system content packs (e.g., D&D 5e, Pathfinder) that
// provide reference data, tooltips, and stat blocks. They are read-only
// and enabled per campaign via campaign settings.
//
// The registry auto-discovers modules by scanning subdirectories for
// manifest.json files at startup. See SystemManifest for the JSON spec.
package systems

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Status represents the implementation status of a module.
type Status string

const (
	// StatusAvailable means the module is fully implemented and ready to enable.
	StatusAvailable Status = "available"

	// StatusComingSoon means the module is planned but not yet implemented.
	StatusComingSoon Status = "coming_soon"
)

// globalLoader is the singleton module loader initialized by Init().
var globalLoader *SystemLoader

// SystemFactory creates a System instance from its manifest and data
// directory. Used by the factory registry to instantiate modules
// without circular imports between the modules package and subpackages.
type SystemFactory func(manifest *SystemManifest, dataDir string) (System, error)

// factories maps module IDs to their factory functions. Subpackages
// register themselves via RegisterFactory in their init() functions.
var factories = make(map[string]SystemFactory)

// RegisterFactory registers a system factory for a given module ID.
// Called from subpackage init() functions (e.g., dnd5e.init()).
func RegisterFactory(id string, factory SystemFactory) {
	factories[id] = factory
}

// Init initializes the system registry by scanning the given directory
// for module subdirectories containing manifest.json files. Must be
// called once at application startup before any Registry()/Find() calls.
func Init(modulesDir string) error {
	globalLoader = NewSystemLoader(modulesDir)
	globalEventLog = NewEventLog(100)
	if err := globalLoader.DiscoverAll(); err != nil {
		return fmt.Errorf("system discovery failed: %w", err)
	}
	if globalLoader.Count() == 0 {
		slog.Info("no bundled systems found, systems will load from package manager")
	} else {
		slog.Info("system registry initialized",
			slog.Int("count", globalLoader.Count()),
		)
	}
	return nil
}

// ScanPackageDir scans the package manager's systems directory for installed
// systems and adds them to the registry. Structure: dir/{slug}/{version}/manifest.json.
// Finds the latest version directory for each slug and loads it.
func ScanPackageDir(dir string) {
	if globalLoader == nil {
		return
	}
	slog.Info("scanning package systems directory", slog.String("dir", dir))
	slugDirs, err := os.ReadDir(dir)
	if err != nil {
		slog.Warn("cannot read package systems directory — no installed systems will load",
			slog.String("dir", dir), slog.Any("error", err))
		return
	}
	slog.Info("found system package slugs", slog.Int("count", len(slugDirs)))
	for _, slugDir := range slugDirs {
		if !slugDir.IsDir() {
			continue
		}
		slugPath := filepath.Join(dir, slugDir.Name())
		// Find the latest version subdirectory.
		versionDirs, err := os.ReadDir(slugPath)
		if err != nil {
			continue
		}
		// Pick the highest SEMVER version directory. os.ReadDir returns names
		// sorted as strings, where "0.10.0" sorts BEFORE "0.0.9" — so taking the
		// last alphabetical name silently loads the wrong (older) package once a
		// double-digit component appears. Compare numerically instead.
		var latestVersion string
		for _, vDir := range versionDirs {
			if !vDir.IsDir() {
				continue
			}
			if latestVersion == "" || versionLess(latestVersion, vDir.Name()) {
				latestVersion = vDir.Name()
			}
		}
		if latestVersion == "" {
			continue
		}
		versionPath := filepath.Join(slugPath, latestVersion)
		if err := globalLoader.DiscoverDir(versionPath); err != nil {
			slog.Warn("failed to load package system",
				slog.String("slug", slugDir.Name()),
				slog.String("version", latestVersion),
				slog.Any("error", err),
			)
			RecordEvent(LoadEvent{
				SystemID: slugDir.Name(),
				Kind:     EventFailed,
				Source:   "package",
				Error:    err.Error(),
				Dir:      versionPath,
			})
		}
	}
	slog.Info("scanned package systems directory",
		slog.String("dir", dir),
		slog.Int("total_count", globalLoader.Count()),
	)
}

// versionLess reports whether version a is lower than version b, comparing
// dot-separated numeric components so "0.0.9" < "0.10.0" (a plain string sort
// gets this backwards). Components that aren't integers compare as 0; on an
// otherwise exact tie it falls back to string comparison so selection stays
// deterministic. Tolerant of arbitrary component counts ("1.2" vs "1.2.3").
func versionLess(a, b string) bool {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var ai, bi int
		if i < len(pa) {
			ai, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			bi, _ = strconv.Atoi(pb[i])
		}
		if ai != bi {
			return ai < bi
		}
	}
	return a < b
}

// Registry returns all discovered module manifests, sorted by name.
// Returns nil if Init() has not been called.
func Registry() []*SystemManifest {
	if globalLoader == nil {
		return nil
	}
	return globalLoader.All()
}

// Find returns the manifest for a given module ID, or nil if not found.
// Returns nil if Init() has not been called.
func Find(id string) *SystemManifest {
	if globalLoader == nil {
		return nil
	}
	return globalLoader.Get(id)
}

// FindSystem returns the live System instance for a given module ID,
// or nil if not found or not yet instantiated. Only modules with
// status "available" have live instances.
func FindSystem(id string) System {
	if globalLoader == nil {
		return nil
	}
	return globalLoader.GetSystem(id)
}

// Dir returns the absolute directory path for a system by ID, or empty string
// if not found. Used to serve system widget JS files from the filesystem.
func Dir(id string) string {
	if globalLoader == nil {
		return ""
	}
	return globalLoader.Dir(id)
}

// AllSystems returns all live System instances, for iteration.
// Only includes modules that have been successfully instantiated.
func AllSystems() []System {
	if globalLoader == nil {
		return nil
	}
	return globalLoader.AllSystems()
}

// AddonInfo contains the metadata needed to register a discovered system
// as an addon in the addons database. This bridges the systems registry
// with the addons plugin so new systems are auto-registered without
// hardcoding addon definitions.
type AddonInfo struct {
	Slug        string
	Name        string
	Description string
	Version     string
	Icon        string
	Author      string
}

// AddonInfos returns addon registration metadata for all discovered
// systems with status "available". Called during app wiring to
// auto-register systems as addons — no hardcoded addon definitions needed.
func AddonInfos() []AddonInfo {
	if globalLoader == nil {
		return nil
	}
	manifests := globalLoader.All()
	infos := make([]AddonInfo, 0, len(manifests))
	for _, m := range manifests {
		if m.Status != StatusAvailable {
			continue
		}
		infos = append(infos, AddonInfo{
			Slug:        m.ID,
			Name:        m.Name,
			Description: m.Description,
			Version:     m.Version,
			Icon:        m.Icon,
			Author:      m.Author,
		})
	}
	slog.Info("system addon infos generated",
		slog.Int("total_manifests", len(manifests)),
		slog.Int("available_count", len(infos)),
	)
	return infos
}

// LoadAdditionalDir scans an additional directory for system manifest.json
// files. This is used by the package manager to load systems from external
// repos installed to media/packages/systems/<slug>/<version>/. Systems
// found here override bundled systems with the same ID.
func LoadAdditionalDir(dir string) error {
	if globalLoader == nil {
		return fmt.Errorf("system registry not initialized")
	}
	return globalLoader.DiscoverDir(dir)
}
