// Package addons manages the extension framework — installable addons
// (modules, widgets, integrations) with per-campaign enable/disable controls.
package addons

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// AddonService handles business logic for addon operations.
// Admin-level methods manage the global registry; campaign-scoped methods
// control which addons are enabled per campaign.
type AddonService interface {
	// Global registry (admin only).
	CountAddons(ctx context.Context) (int, error)
	CountFeatures(ctx context.Context) (int, error)
	List(ctx context.Context) ([]Addon, error)
	GetByID(ctx context.Context, id int) (*Addon, error)
	GetBySlug(ctx context.Context, slug string) (*Addon, error)
	Create(ctx context.Context, input CreateAddonInput) (*Addon, error)
	Update(ctx context.Context, id int, input UpdateAddonInput) (*Addon, error)
	Delete(ctx context.Context, id int) error
	UpdateStatus(ctx context.Context, id int, status AddonStatus) error

	// SeedInstalledAddons upserts all built-in addons at startup.
	SeedInstalledAddons(ctx context.Context) error

	// Per-campaign controls (campaign owner).
	ListForCampaign(ctx context.Context, campaignID string) ([]CampaignAddon, error)
	EnableForCampaign(ctx context.Context, campaignID string, addonID int, userID string) error
	DisableForCampaign(ctx context.Context, campaignID string, addonID int) error
	IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error)
	UpdateCampaignConfig(ctx context.Context, campaignID string, addonID int, config map[string]any) error

	// SetPresetApplier injects the preset applier for auto-creating entity
	// types when a game system addon is enabled. Optional — if nil, no
	// presets are applied.
	SetPresetApplier(applier PresetApplier)
}

// PresetApplier creates entity types from system presets when a game system
// addon is enabled for a campaign. Implemented outside the addons package
// to avoid circular imports with entities and systems packages.
type PresetApplier interface {
	ApplySystemPresets(ctx context.Context, campaignID, systemSlug string) (int, error)
}

// addonService implements AddonService.
type addonService struct {
	repo           AddonRepository
	presetApplier  PresetApplier
}

// NewAddonService creates a new addon service.
func NewAddonService(repo AddonRepository) AddonService {
	return &addonService{repo: repo}
}

// SetPresetApplier injects the preset applier for auto-creating entity types
// when a game system is enabled. Optional — if nil, presets are not applied.
func (s *addonService) SetPresetApplier(applier PresetApplier) {
	s.presetApplier = applier
}

// CountAddons returns the total number of registered addons.
func (s *addonService) CountAddons(ctx context.Context) (int, error) {
	return s.repo.Count(ctx)
}

// CountFeatures returns the number of feature addons visible on the admin
// Features page. Excludes module-category game systems (Content Packs page)
// and planned addons without backing code (future work, not actionable).
func (s *addonService) CountFeatures(ctx context.Context) (int, error) {
	addons, err := s.repo.List(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, a := range addons {
		if a.Category == CategorySystem {
			continue
		}
		if a.Status == StatusPlanned && !installedAddons[a.Slug] {
			continue
		}
		count++
	}
	return count, nil
}

// List returns all registered addons with installation status annotated.
func (s *addonService) List(ctx context.Context) ([]Addon, error) {
	addons, err := s.repo.List(ctx)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing addons: %w", err))
	}
	for i := range addons {
		addons[i].Installed = IsInstalled(addons[i].Slug)
	}
	return addons, nil
}

// GetByID retrieves an addon by its ID.
func (s *addonService) GetByID(ctx context.Context, id int) (*Addon, error) {
	addon, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err // Already an apperror from repo.
	}
	return addon, nil
}

// GetBySlug retrieves an addon by its slug.
func (s *addonService) GetBySlug(ctx context.Context, slug string) (*Addon, error) {
	addon, err := s.repo.FindBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	return addon, nil
}

// validCategories enumerates allowed addon categories for input validation.
// Must stay in sync with the ENUM on addons.category in the database.
// See db/migrations/000001_baseline.up.sql for the current ENUM definition.
var validCategories = map[AddonCategory]bool{
	CategorySystem:      true,
	CategoryWidget:      true,
	CategoryIntegration: true,
	CategoryPlugin:      true,
}

// validStatuses enumerates allowed addon statuses for input validation.
var validStatuses = map[AddonStatus]bool{
	StatusActive:     true,
	StatusPlanned:    true,
	StatusDeprecated: true,
}

// addonDef describes a built-in addon that ships with the codebase.
// Used for automatic registration at startup — no migration needed.
type addonDef struct {
	Slug        string
	Name        string
	Description string
	Version     string
	Category    AddonCategory
	Status      AddonStatus
	Icon        string
	Author      string
}

// builtinAddons is the canonical registry of all addons that ship with
// Chronicle. Adding a new addon here is all that's needed — the startup
// seeder will upsert it into the database automatically. No migration required.
var builtinAddons = []addonDef{
	// Game systems (content packs).
	{Slug: "dnd5e", Name: "D&D 5th Edition", Description: "Reference data, stat blocks, and tooltips for Dungeons & Dragons 5th Edition", Version: "0.1.0", Category: CategorySystem, Status: StatusActive, Icon: "fa-dragon", Author: "Chronicle"},
	{Slug: "pathfinder2e", Name: "Pathfinder 2nd Edition", Description: "Reference data and tooltips for Pathfinder 2nd Edition", Version: "0.1.0", Category: CategorySystem, Status: StatusActive, Icon: "fa-shield-halved", Author: "Chronicle"},
	{Slug: "drawsteel", Name: "Draw Steel", Description: "Reference data for the Draw Steel RPG system", Version: "0.1.0", Category: CategorySystem, Status: StatusActive, Icon: "fa-swords", Author: "Chronicle"},

	// Plugins (feature apps).
	{Slug: "calendar", Name: "Calendar", Description: "Custom fantasy calendar with configurable months, weekdays, moons, seasons, and events. Link events to entities for timeline tracking.", Version: "0.1.0", Category: CategoryPlugin, Status: StatusActive, Icon: "fa-calendar-days", Author: "Chronicle"},
	{Slug: "maps", Name: "Interactive Maps", Description: "Leaflet.js map viewer with entity pins and layer support", Version: "0.1.0", Category: CategoryPlugin, Status: StatusActive, Icon: "fa-map", Author: "Chronicle"},
	{Slug: "media-gallery", Name: "Media Gallery", Description: "Campaign media management — upload, browse, and organize images.", Version: "0.1.0", Category: CategoryPlugin, Status: StatusActive, Icon: "fa-images", Author: "Chronicle"},
	{Slug: "timeline", Name: "Timeline", Description: "Interactive visual timelines with zoom levels, entity grouping, and calendar integration.", Version: "0.1.0", Category: CategoryPlugin, Status: StatusActive, Icon: "fa-timeline", Author: "Chronicle"},
	{Slug: "sessions", Name: "Sessions", Description: "Track game sessions with scheduling, linked entities, and RSVP.", Version: "0.1.0", Category: CategoryPlugin, Status: StatusActive, Icon: "fa-calendar-check", Author: "Chronicle"},
	{Slug: "npcs", Name: "NPC Gallery", Description: "Browse and reveal character entities as NPCs for your players.", Version: "1.0.0", Category: CategoryPlugin, Status: StatusActive, Icon: "fa-users", Author: "Chronicle"},
	{Slug: "armory", Name: "Armory & Inventory", Description: "Item catalog, character inventories, and shop management. System-dependent item types with Foundry sync.", Version: "0.1.0", Category: CategoryPlugin, Status: StatusActive, Icon: "fa-shield-halved", Author: "Chronicle"},

	// Integrations.
	{Slug: "sync-api", Name: "Sync API", Description: "Secure REST API for external tool integration (Foundry VTT, Roll20, etc.)", Version: "0.1.0", Category: CategoryIntegration, Status: StatusActive, Icon: "fa-arrows-rotate", Author: "Chronicle"},

	// Widgets.
	{Slug: "notes", Name: "Notes", Description: "Floating notebook panel for personal and shared campaign notes. Includes checklists, color coding, version history, and edit locking.", Version: "0.1.0", Category: CategoryWidget, Status: StatusActive, Icon: "fa-book", Author: "Chronicle"},
	{Slug: "attributes", Name: "Attributes", Description: "Custom attribute fields on entity pages (e.g. race, alignment, HP). When disabled, attribute panels are hidden.", Version: "0.1.0", Category: CategoryWidget, Status: StatusActive, Icon: "fa-sliders", Author: "Chronicle"},

	// Planned (no backing code yet).
	{Slug: "player-notes", Name: "Player Notes", Description: "Collaborative note-taking block for entity pages.", Version: "0.1.0", Category: CategoryWidget, Status: StatusPlanned, Icon: "fa-sticky-note", Author: "Chronicle"},
	{Slug: "family-tree", Name: "Family Tree", Description: "Visual family/org tree diagram from entity relations", Version: "0.1.0", Category: CategoryWidget, Status: StatusPlanned, Icon: "fa-sitemap", Author: "Chronicle"},
	{Slug: "dice-roller", Name: "Dice Roller", Description: "In-app dice rolling with formula support and history", Version: "0.1.0", Category: CategoryWidget, Status: StatusPlanned, Icon: "fa-dice-d20", Author: "Chronicle"},
}

// installedAddons is derived from builtinAddons for quick lookup.
var installedAddons map[string]bool

func init() {
	installedAddons = make(map[string]bool, len(builtinAddons))
	for _, a := range builtinAddons {
		if a.Status == StatusActive {
			installedAddons[a.Slug] = true
		}
	}
}

// IsInstalled reports whether an addon slug has backing code in the codebase.
func IsInstalled(slug string) bool {
	return installedAddons[slug]
}

// SeedInstalledAddons upserts all built-in addons into the database.
// Called once at startup so new addons are registered automatically
// without requiring SQL migrations.
func (s *addonService) SeedInstalledAddons(ctx context.Context) error {
	for _, def := range builtinAddons {
		desc := def.Description
		author := def.Author
		addon := &Addon{
			Slug:        def.Slug,
			Name:        def.Name,
			Description: &desc,
			Version:     def.Version,
			Category:    def.Category,
			Status:      def.Status,
			Icon:        def.Icon,
			Author:      &author,
		}
		if err := s.repo.Upsert(ctx, addon); err != nil {
			return fmt.Errorf("seeding addon %s: %w", def.Slug, err)
		}
	}
	slog.Info("built-in addons registered", slog.Int("count", len(builtinAddons)))
	return nil
}

// Create registers a new addon in the global registry.
func (s *addonService) Create(ctx context.Context, input CreateAddonInput) (*Addon, error) {
	slug := strings.TrimSpace(input.Slug)
	name := strings.TrimSpace(input.Name)

	if slug == "" {
		return nil, apperror.NewBadRequest("addon slug is required")
	}
	if name == "" {
		return nil, apperror.NewBadRequest("addon name is required")
	}
	if !validCategories[input.Category] {
		return nil, apperror.NewBadRequest("invalid addon category")
	}

	// Check slug uniqueness.
	existing, err := s.repo.FindBySlug(ctx, slug)
	if err == nil && existing != nil {
		return nil, apperror.NewConflict("an addon with this slug already exists")
	}

	addon := &Addon{
		Slug:     slug,
		Name:     name,
		Version:  strings.TrimSpace(input.Version),
		Category: input.Category,
		Status:   StatusPlanned, // New addons start as planned.
		Icon:     strings.TrimSpace(input.Icon),
	}
	if desc := strings.TrimSpace(input.Description); desc != "" {
		addon.Description = &desc
	}
	if author := strings.TrimSpace(input.Author); author != "" {
		addon.Author = &author
	}

	if err := s.repo.Create(ctx, addon); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating addon: %w", err))
	}

	slog.Info("addon created",
		slog.String("slug", addon.Slug),
		slog.Int("id", addon.ID),
	)
	return addon, nil
}

// Update modifies an addon's metadata.
func (s *addonService) Update(ctx context.Context, id int, input UpdateAddonInput) (*Addon, error) {
	addon, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("addon name is required")
	}
	if !validStatuses[input.Status] {
		return nil, apperror.NewBadRequest("invalid addon status")
	}

	addon.Name = name
	addon.Version = strings.TrimSpace(input.Version)
	addon.Status = input.Status
	addon.Icon = strings.TrimSpace(input.Icon)
	if desc := strings.TrimSpace(input.Description); desc != "" {
		addon.Description = &desc
	} else {
		addon.Description = nil
	}

	if err := s.repo.Update(ctx, addon); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("updating addon: %w", err))
	}

	slog.Info("addon updated",
		slog.String("slug", addon.Slug),
		slog.Int("id", addon.ID),
	)
	return addon, nil
}

// Delete removes an addon from the registry.
func (s *addonService) Delete(ctx context.Context, id int) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	slog.Info("addon deleted", slog.Int("id", id))
	return nil
}

// UpdateStatus changes an addon's lifecycle status. Activating an addon
// requires its code to be installed — uninstalled addons cannot be active.
func (s *addonService) UpdateStatus(ctx context.Context, id int, status AddonStatus) error {
	if !validStatuses[status] {
		return apperror.NewBadRequest("invalid addon status")
	}

	// Block activating addons that have no backing code.
	if status == StatusActive {
		addon, err := s.repo.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if !IsInstalled(addon.Slug) {
			return apperror.NewBadRequest("cannot activate: extension code is not installed")
		}
	}

	if err := s.repo.UpdateStatus(ctx, id, status); err != nil {
		return err
	}
	slog.Info("addon status updated",
		slog.Int("id", id),
		slog.String("status", string(status)),
	)
	return nil
}

// ListForCampaign returns all active addons with their per-campaign enabled
// state and installation status annotated.
func (s *addonService) ListForCampaign(ctx context.Context, campaignID string) ([]CampaignAddon, error) {
	addons, err := s.repo.ListForCampaign(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing campaign addons: %w", err))
	}
	for i := range addons {
		addons[i].Installed = IsInstalled(addons[i].AddonSlug)
	}
	return addons, nil
}

// EnableForCampaign enables an addon for a campaign. Game systems (module
// category) are mutually exclusive — enabling one auto-disables any other
// active game system for that campaign.
func (s *addonService) EnableForCampaign(ctx context.Context, campaignID string, addonID int, userID string) error {
	// Verify addon exists, is active, and has backing code.
	addon, err := s.repo.FindByID(ctx, addonID)
	if err != nil {
		return err
	}
	if addon.Status != StatusActive {
		return apperror.NewBadRequest("only active addons can be enabled")
	}
	if !IsInstalled(addon.Slug) {
		return apperror.NewBadRequest("cannot enable: extension code is not installed")
	}

	// Game systems are mutually exclusive — disable any other enabled module.
	if addon.Category == CategorySystem {
		campaignAddons, err := s.repo.ListForCampaign(ctx, campaignID)
		if err != nil {
			return apperror.NewInternal(fmt.Errorf("listing campaign addons: %w", err))
		}
		for _, ca := range campaignAddons {
			if ca.AddonCategory == CategorySystem && ca.Enabled && ca.AddonID != addonID {
				if err := s.repo.DisableForCampaign(ctx, campaignID, ca.AddonID); err != nil {
					return apperror.NewInternal(fmt.Errorf("disabling previous game system: %w", err))
				}
				slog.Info("auto-disabled game system (mutual exclusivity)",
					slog.String("campaign_id", campaignID),
					slog.String("disabled_slug", ca.AddonSlug),
					slog.String("replacing_with", addon.Slug),
				)
			}
		}
	}

	if err := s.repo.EnableForCampaign(ctx, campaignID, addonID, userID); err != nil {
		return apperror.NewInternal(fmt.Errorf("enabling addon: %w", err))
	}

	// Auto-create entity types from system presets when a game system is enabled.
	if addon.Category == CategorySystem && s.presetApplier != nil {
		count, err := s.presetApplier.ApplySystemPresets(ctx, campaignID, addon.Slug)
		if err != nil {
			slog.Warn("failed to apply system presets (addon still enabled)",
				slog.String("campaign_id", campaignID),
				slog.String("system_slug", addon.Slug),
				slog.Any("error", err),
			)
		} else if count > 0 {
			slog.Info("system presets applied",
				slog.String("campaign_id", campaignID),
				slog.String("system_slug", addon.Slug),
				slog.Int("types_created", count),
			)
		}
	}

	slog.Info("addon enabled for campaign",
		slog.String("campaign_id", campaignID),
		slog.String("addon_slug", addon.Slug),
		slog.String("user_id", userID),
	)
	return nil
}

// DisableForCampaign disables an addon for a campaign.
func (s *addonService) DisableForCampaign(ctx context.Context, campaignID string, addonID int) error {
	if err := s.repo.DisableForCampaign(ctx, campaignID, addonID); err != nil {
		return apperror.NewInternal(fmt.Errorf("disabling addon: %w", err))
	}
	slog.Info("addon disabled for campaign",
		slog.String("campaign_id", campaignID),
		slog.Int("addon_id", addonID),
	)
	return nil
}

// IsEnabledForCampaign checks if a specific addon is enabled for a campaign.
func (s *addonService) IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error) {
	return s.repo.IsEnabledForCampaign(ctx, campaignID, addonSlug)
}

// UpdateCampaignConfig updates the addon-specific configuration for a campaign.
func (s *addonService) UpdateCampaignConfig(ctx context.Context, campaignID string, addonID int, config map[string]any) error {
	if err := s.repo.UpdateCampaignConfig(ctx, campaignID, addonID, config); err != nil {
		return apperror.NewInternal(fmt.Errorf("updating campaign addon config: %w", err))
	}
	return nil
}
