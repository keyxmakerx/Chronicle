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
	List(ctx context.Context) ([]Addon, error)
	GetByID(ctx context.Context, id int) (*Addon, error)
	GetBySlug(ctx context.Context, slug string) (*Addon, error)
	Create(ctx context.Context, input CreateAddonInput) (*Addon, error)
	Update(ctx context.Context, id int, input UpdateAddonInput) (*Addon, error)
	Delete(ctx context.Context, id int) error
	UpdateStatus(ctx context.Context, id int, status AddonStatus) error

	// Per-campaign controls (campaign owner).
	ListForCampaign(ctx context.Context, campaignID string) ([]CampaignAddon, error)
	EnableForCampaign(ctx context.Context, campaignID string, addonID int, userID string) error
	DisableForCampaign(ctx context.Context, campaignID string, addonID int) error
	IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error)
	UpdateCampaignConfig(ctx context.Context, campaignID string, addonID int, config map[string]any) error
}

// addonService implements AddonService.
type addonService struct {
	repo AddonRepository
}

// NewAddonService creates a new addon service.
func NewAddonService(repo AddonRepository) AddonService {
	return &addonService{repo: repo}
}

// CountAddons returns the total number of registered addons.
func (s *addonService) CountAddons(ctx context.Context) (int, error) {
	return s.repo.Count(ctx)
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
// See migration 000027 for the ALTER TABLE that added 'plugin'.
var validCategories = map[AddonCategory]bool{
	CategoryModule:      true,
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

// installedAddons lists addon slugs that have real backing code in the
// codebase. Only installed addons can be activated by admins or enabled
// by campaign owners. Update this set as new addons are built.
var installedAddons = map[string]bool{
	"sync-api":   true,
	"notes":      true,
	"attributes": true,
	"calendar":   true,
	"maps":       true,
	"sessions":   true,
}

// IsInstalled reports whether an addon slug has backing code in the codebase.
func IsInstalled(slug string) bool {
	return installedAddons[slug]
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

// EnableForCampaign enables an addon for a campaign.
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

	if err := s.repo.EnableForCampaign(ctx, campaignID, addonID, userID); err != nil {
		return apperror.NewInternal(fmt.Errorf("enabling addon: %w", err))
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
