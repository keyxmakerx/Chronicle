// Package modules defines the module registry for Chronicle.
// Modules are game-system content packs (e.g., D&D 5e, Pathfinder) that
// provide reference data, tooltips, and stat blocks. They are read-only
// and enabled per campaign via campaign settings.
package modules

// Status represents the implementation status of a module.
type Status string

const (
	// StatusAvailable means the module is fully implemented and ready to enable.
	StatusAvailable Status = "available"

	// StatusComingSoon means the module is planned but not yet implemented.
	StatusComingSoon Status = "coming_soon"
)

// ModuleInfo holds metadata about a registered module.
type ModuleInfo struct {
	// ID is the unique machine-readable identifier (e.g., "dnd5e").
	ID string

	// Name is the human-readable display name.
	Name string

	// Description is a short summary of what the module provides.
	Description string

	// Icon is the Font Awesome icon class (e.g., "fa-dragon").
	Icon string

	// Version is the current version string (empty for unimplemented modules).
	Version string

	// Status indicates whether the module is available or coming soon.
	Status Status

	// Categories lists the types of reference content provided.
	Categories []string
}

// Registry returns the list of all known modules, both implemented and planned.
// This is the canonical source of truth for what modules exist in Chronicle.
func Registry() []ModuleInfo {
	return []ModuleInfo{
		{
			ID:          "dnd5e",
			Name:        "D&D 5th Edition",
			Description: "SRD reference content for Dungeons & Dragons 5th Edition. Includes spells, monsters, items, classes, and more from the System Reference Document.",
			Icon:        "fa-dragon",
			Version:     "",
			Status:      StatusComingSoon,
			Categories:  []string{"Spells", "Monsters", "Items", "Classes", "Races"},
		},
		{
			ID:          "pathfinder2e",
			Name:        "Pathfinder 2e",
			Description: "ORC reference content for Pathfinder 2nd Edition. Includes spells, creatures, equipment, ancestries, and class features.",
			Icon:        "fa-shield-halved",
			Version:     "",
			Status:      StatusComingSoon,
			Categories:  []string{"Spells", "Creatures", "Equipment", "Ancestries"},
		},
		{
			ID:          "drawsteel",
			Name:        "Draw Steel",
			Description: "Reference content for the Draw Steel RPG by MCDM Productions. Includes abilities, creatures, and ancestries.",
			Icon:        "fa-bolt",
			Version:     "",
			Status:      StatusComingSoon,
			Categories:  []string{"Abilities", "Creatures", "Ancestries"},
		},
	}
}

// Find returns the module info for a given ID, or nil if not found.
func Find(id string) *ModuleInfo {
	for _, m := range Registry() {
		if m.ID == id {
			return &m
		}
	}
	return nil
}
