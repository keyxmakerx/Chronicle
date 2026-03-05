// Package modules defines the module registry for Chronicle.
// Modules are game-system content packs (e.g., D&D 5e, Pathfinder) that
// provide reference data, tooltips, and stat blocks. They are read-only
// and enabled per campaign via campaign settings.
//
// The registry auto-discovers modules by scanning subdirectories for
// manifest.json files at startup. See ModuleManifest for the JSON spec.
package modules

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
var globalLoader *ModuleLoader

// ModuleFactory creates a Module instance from its manifest and data
// directory. Used by the factory registry to instantiate modules
// without circular imports between the modules package and subpackages.
type ModuleFactory func(manifest *ModuleManifest, dataDir string) (Module, error)

// factories maps module IDs to their factory functions. Subpackages
// register themselves via RegisterFactory in their init() functions.
var factories = make(map[string]ModuleFactory)

// RegisterFactory registers a module factory for a given module ID.
// Called from subpackage init() functions (e.g., dnd5e.init()).
func RegisterFactory(id string, factory ModuleFactory) {
	factories[id] = factory
}

// Init initializes the module registry by scanning the given directory
// for module subdirectories containing manifest.json files. Must be
// called once at application startup before any Registry()/Find() calls.
func Init(modulesDir string) error {
	globalLoader = NewModuleLoader(modulesDir)
	if err := globalLoader.DiscoverAll(); err != nil {
		return fmt.Errorf("module discovery failed: %w", err)
	}
	slog.Info("module registry initialized",
		slog.Int("count", globalLoader.Count()),
	)
	return nil
}

// Registry returns all discovered module manifests, sorted by name.
// Returns nil if Init() has not been called.
func Registry() []*ModuleManifest {
	if globalLoader == nil {
		return nil
	}
	return globalLoader.All()
}

// Find returns the manifest for a given module ID, or nil if not found.
// Returns nil if Init() has not been called.
func Find(id string) *ModuleManifest {
	if globalLoader == nil {
		return nil
	}
	return globalLoader.Get(id)
}

// FindModule returns the live Module instance for a given module ID,
// or nil if not found or not yet instantiated. Only modules with
// status "available" have live instances.
func FindModule(id string) Module {
	if globalLoader == nil {
		return nil
	}
	return globalLoader.GetModule(id)
}

// AllModules returns all live Module instances, for iteration.
// Only includes modules that have been successfully instantiated.
func AllModules() []Module {
	if globalLoader == nil {
		return nil
	}
	return globalLoader.AllModules()
}
