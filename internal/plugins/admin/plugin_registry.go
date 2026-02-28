// plugin_registry.go defines metadata for Chronicle's built-in plugins.
// Plugins are feature applications that provide core functionality.
// Unlike modules (game-system content packs), plugins provide infrastructure
// and workflow features.

package admin

// PluginStatus represents the state of a plugin in the system.
type PluginStatus string

const (
	// PluginActive means the plugin is installed and running.
	PluginActive PluginStatus = "active"

	// PluginPlanned means the plugin is planned but not yet built.
	PluginPlanned PluginStatus = "planned"
)

// PluginInfo holds metadata about a registered plugin.
type PluginInfo struct {
	// ID is the unique machine-readable identifier (e.g., "auth").
	ID string

	// Name is the human-readable display name.
	Name string

	// Description is a short summary of what the plugin provides.
	Description string

	// Icon is the Font Awesome icon class (e.g., "fa-shield-halved").
	Icon string

	// Status indicates whether the plugin is active or planned.
	Status PluginStatus

	// Category groups related plugins (e.g., "Core", "Content", "Integration").
	Category string
}

// PluginRegistry returns the list of all known plugins, both built and planned.
func PluginRegistry() []PluginInfo {
	return []PluginInfo{
		{
			ID:          "auth",
			Name:        "Authentication",
			Description: "User registration, login, logout, session management, and TOTP two-factor authentication.",
			Icon:        "fa-shield-halved",
			Status:      PluginActive,
			Category:    "Core",
		},
		{
			ID:          "campaigns",
			Name:        "Campaigns",
			Description: "Campaign and world management with roles (owner, scribe, player), membership, and ownership transfer.",
			Icon:        "fa-book-open",
			Status:      PluginActive,
			Category:    "Core",
		},
		{
			ID:          "entities",
			Name:        "Entities",
			Description: "Universal entity system with configurable types, custom fields, FULLTEXT search, and privacy controls.",
			Icon:        "fa-scroll",
			Status:      PluginActive,
			Category:    "Content",
		},
		{
			ID:          "media",
			Name:        "Media",
			Description: "File upload with magic byte validation, thumbnail generation, rate limiting, and storage limits.",
			Icon:        "fa-image",
			Status:      PluginActive,
			Category:    "Content",
		},
		{
			ID:          "audit",
			Name:        "Audit Log",
			Description: "Campaign-scoped activity timeline tracking entity, tag, and membership mutations.",
			Icon:        "fa-clock-rotate-left",
			Status:      PluginActive,
			Category:    "Core",
		},
		{
			ID:          "settings",
			Name:        "Site Settings",
			Description: "Global storage limits, per-user and per-campaign overrides, and site configuration.",
			Icon:        "fa-sliders",
			Status:      PluginActive,
			Category:    "Core",
		},
		{
			ID:          "smtp",
			Name:        "SMTP Email",
			Description: "Email configuration with AES-256-GCM encrypted credentials, STARTTLS/SSL support.",
			Icon:        "fa-envelope",
			Status:      PluginActive,
			Category:    "Integration",
		},
		{
			ID:          "admin",
			Name:        "Administration",
			Description: "Site-wide dashboard, user management, campaign oversight, storage management, and module registry.",
			Icon:        "fa-screwdriver-wrench",
			Status:      PluginActive,
			Category:    "Core",
		},
		{
			ID:          "maps",
			Name:        "Maps",
			Description: "Interactive map viewer with entity pins and region overlays using Leaflet.js.",
			Icon:        "fa-map",
			Status:      PluginActive,
			Category:    "Content",
		},
		{
			ID:          "calendar",
			Name:        "Calendar",
			Description: "Custom calendar system for tracking in-world dates, events, and timelines.",
			Icon:        "fa-calendar-days",
			Status:      PluginPlanned,
			Category:    "Content",
		},
		{
			ID:          "api",
			Name:        "REST API",
			Description: "External API with PASETO token authentication for third-party integrations.",
			Icon:        "fa-plug",
			Status:      PluginPlanned,
			Category:    "Integration",
		},
	}
}
