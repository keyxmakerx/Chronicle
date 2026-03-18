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

// AllSystems returns all live System instances, for iteration.
// Only includes modules that have been successfully instantiated.
func AllSystems() []System {
	if globalLoader == nil {
		return nil
	}
	return globalLoader.AllSystems()
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
