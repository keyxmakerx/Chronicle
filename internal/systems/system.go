// Package modules defines the system framework for Chronicle.
// Modules are read-only game system content packs that provide
// reference data (spells, monsters, items, stat blocks) and tooltip
// rendering. They implement sandboxed interfaces and never access
// the database, Echo router, or plugin services directly.
package systems

// System is the sandboxed interface that every game system content pack
// must implement. It provides metadata, reference data access, and
// tooltip rendering without any access to Chronicle's infrastructure.
type System interface {
	// Info returns the module's manifest metadata.
	Info() *SystemManifest

	// DataProvider returns the module's reference data provider, or nil
	// if the system has no data loaded yet (coming_soon modules).
	DataProvider() DataProvider

	// TooltipRenderer returns the module's tooltip renderer, or nil
	// if the module has no tooltip templates yet.
	TooltipRenderer() TooltipRenderer
}

// DataProvider serves read-only reference data from a module.
// Implementations load data from static JSON files at startup and
// serve it from memory. They never access the database.
type DataProvider interface {
	// List returns all reference items in the given category.
	List(category string) ([]ReferenceItem, error)

	// Get returns a single reference item by category and ID (slug).
	// Returns nil and no error if the item does not exist.
	Get(category string, id string) (*ReferenceItem, error)

	// Search returns reference items matching the query string
	// across all categories. Used for tooltip lookups and autocomplete.
	Search(query string) ([]ReferenceItem, error)

	// Categories returns the list of available data category slugs.
	Categories() []string
}

// TooltipRenderer produces HTML tooltip fragments for reference items.
// Implementations use Go templates or Templ components to render
// hover-preview content for @mention popups.
type TooltipRenderer interface {
	// RenderTooltip returns an HTML fragment for the given reference item.
	RenderTooltip(item *ReferenceItem) (string, error)

	// SupportedCategories returns which category slugs this renderer handles.
	SupportedCategories() []string
}

// ReferenceItem is the standard struct for all system reference data.
// It provides a uniform shape regardless of game system, with
// system-specific details in the Properties map.
type ReferenceItem struct {
	// ID is the unique identifier within this category (typically a slug).
	ID string `json:"id"`

	// Category is the content type slug (e.g., "spells", "monsters", "items").
	Category string `json:"category"`

	// Name is the human-readable display name.
	Name string `json:"name"`

	// Summary is a short one-line description for list views and search results.
	Summary string `json:"summary"`

	// Description is the full reference text (may contain markdown or HTML).
	Description string `json:"description,omitempty"`

	// Properties holds system-specific fields (e.g., spell level, monster CR,
	// item weight). Keys are lowercase snake_case.
	Properties map[string]any `json:"properties,omitempty"`

	// Tags are searchable labels (e.g., "evocation", "fire", "cantrip").
	Tags []string `json:"tags,omitempty"`

	// Source identifies where this data comes from (e.g., "SRD 5.1", "ORC").
	Source string `json:"source,omitempty"`

	// SystemID identifies which module owns this item.
	SystemID string `json:"system_id"`
}
