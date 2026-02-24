// Package addons manages the extension framework — installable addons
// (modules, widgets, integrations) with per-campaign enable/disable controls.
// Admin manages the global addon registry; campaign owners toggle addons per campaign.
package addons

import "time"

// AddonCategory classifies what kind of addon this is.
type AddonCategory string

const (
	CategoryModule      AddonCategory = "module"
	CategoryWidget      AddonCategory = "widget"
	CategoryIntegration AddonCategory = "integration"
)

// AddonStatus tracks the lifecycle state of an addon.
type AddonStatus string

const (
	StatusActive     AddonStatus = "active"
	StatusPlanned    AddonStatus = "planned"
	StatusDeprecated AddonStatus = "deprecated"
)

// Addon represents a registered extension in the global addon registry.
type Addon struct {
	ID           int            `json:"id"`
	Slug         string         `json:"slug"`
	Name         string         `json:"name"`
	Description  *string        `json:"description,omitempty"`
	Version      string         `json:"version"`
	Category     AddonCategory  `json:"category"`
	Status       AddonStatus    `json:"status"`
	Icon         string         `json:"icon"`
	Author       *string        `json:"author,omitempty"`
	ConfigSchema map[string]any `json:"config_schema,omitempty"` // JSON schema for addon-specific config.
	Installed    bool           `json:"installed"`                // Whether backing code exists (set by service, not persisted).
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// CampaignAddon represents a per-campaign addon configuration.
type CampaignAddon struct {
	ID         int            `json:"id"`
	CampaignID string         `json:"campaign_id"`
	AddonID    int            `json:"addon_id"`
	Enabled    bool           `json:"enabled"`
	ConfigJSON map[string]any `json:"config_json,omitempty"` // Addon-specific campaign config.
	EnabledAt  time.Time      `json:"enabled_at"`
	EnabledBy  *string        `json:"enabled_by,omitempty"`

	// Joined from addons table.
	AddonSlug     string        `json:"addon_slug,omitempty"`
	AddonName     string        `json:"addon_name,omitempty"`
	AddonIcon     string        `json:"addon_icon,omitempty"`
	AddonCategory AddonCategory `json:"addon_category,omitempty"`
	AddonStatus   AddonStatus   `json:"addon_status,omitempty"`
	Installed     bool          `json:"installed"` // Whether backing code exists (set by service, not persisted).
}

// CreateAddonInput is the validated input for registering a new addon.
type CreateAddonInput struct {
	Slug        string
	Name        string
	Description string
	Version     string
	Category    AddonCategory
	Icon        string
	Author      string
}

// UpdateAddonInput is the validated input for updating an addon's metadata.
type UpdateAddonInput struct {
	Name        string
	Description string
	Version     string
	Status      AddonStatus
	Icon        string
}
