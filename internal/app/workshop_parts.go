package app

import (
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/fieldrules"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/systemstate"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// workshopDeps is what a feature may need to plug parts, events or actions
// into the Homebrew Workshop. A field is nil while its plugin is degraded.
type workshopDeps struct {
	Entities   entities.EntityService
	FieldRules *fieldRulesWorld
	// FieldRuleStore saves the rules and words field parts describe; nil
	// while the field rules are degraded.
	FieldRuleStore fieldrules.Service
	Health         *database.PluginHealthRegistry
	SystemEntries  systems.SystemEntryService
	SystemState    systemstate.Service
	Systems        *systems.SystemHandler
	Trackers       trackers.Service
}

// workshopSource is one feature's Workshop registration. Label names the
// feature in the error logged when it cannot join.
type workshopSource struct {
	Label    string
	Register func(reg *homebrew.Registry, d workshopDeps) error
}

// workshopSources is filled by each feature's own init through
// addWorkshopSource, so a new feature adds a file instead of editing
// RegisterRoutes or a shared list. Order does not matter: the Workshop
// catalog is sorted by name.
var workshopSources []workshopSource

func addWorkshopSource(label string, register func(reg *homebrew.Registry, d workshopDeps) error) {
	workshopSources = append(workshopSources, workshopSource{Label: label, Register: register})
}

// registerWorkshopParts runs every source. One that fails is logged and left
// out, and the rest still join.
func registerWorkshopParts(reg *homebrew.Registry, d workshopDeps) {
	for _, s := range workshopSources {
		if err := s.Register(reg, d); err != nil {
			slog.Error(s.Label+" could not join the Homebrew Workshop", slog.Any("error", err))
		}
	}
}
