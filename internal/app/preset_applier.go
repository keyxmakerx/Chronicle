// Package app wires together all application dependencies.
// This file implements the addons.PresetApplier interface, bridging the
// systems package (manifest data) and entities package (entity type creation)
// to auto-create entity types when a game system addon is enabled.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// presetApplier implements addons.PresetApplier by looking up system manifests
// and creating entity types from their preset definitions.
type presetApplier struct {
	entityService entities.EntityService
}

// newPresetApplier creates a PresetApplier that bridges systems and entities.
func newPresetApplier(entityService entities.EntityService) *presetApplier {
	return &presetApplier{entityService: entityService}
}

// ApplySystemPresets looks up the system manifest by slug and creates entity
// types from its presets. Skips presets whose category already exists in the
// campaign (avoids duplicates on re-enable). Returns the count of newly
// created entity types.
func (p *presetApplier) ApplySystemPresets(ctx context.Context, campaignID, systemSlug string) (int, error) {
	manifest := systems.Find(systemSlug)
	if manifest == nil {
		// System not found in registry — may be a custom upload without
		// bundled manifest. Not an error, just nothing to apply.
		return 0, nil
	}

	if len(manifest.EntityPresets) == 0 {
		return 0, nil
	}

	// Get existing entity types to avoid duplicate creation.
	existingTypes, err := p.entityService.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return 0, fmt.Errorf("listing existing types: %w", err)
	}

	// Build set of existing preset categories to skip duplicates.
	existingCategories := make(map[string]bool, len(existingTypes))
	for _, et := range existingTypes {
		if et.PresetCategory != nil {
			existingCategories[*et.PresetCategory] = true
		}
	}

	// Build set of existing names to catch entity types created before
	// preset_category was introduced or created manually by the user.
	existingNames := make(map[string]bool, len(existingTypes))
	for _, et := range existingTypes {
		existingNames[strings.ToLower(et.Name)] = true
	}

	created := 0
	for _, preset := range manifest.EntityPresets {
		// Skip if this category already has an entity type.
		if preset.Category != "" && existingCategories[preset.Category] {
			slog.Debug("skipping preset (category already exists)",
				slog.String("campaign_id", campaignID),
				slog.String("preset", preset.Slug),
				slog.String("category", preset.Category),
			)
			continue
		}

		// Skip if an entity type with the same name already exists (catches
		// types created before preset_category was added or manually created).
		if existingNames[strings.ToLower(preset.Name)] {
			slog.Debug("skipping preset (name already exists)",
				slog.String("campaign_id", campaignID),
				slog.String("preset", preset.Slug),
				slog.String("name", preset.Name),
			)
			continue
		}

		input := entities.CreateEntityTypeInput{
			Name:           preset.Name,
			NamePlural:     preset.NamePlural,
			Icon:           preset.Icon,
			Color:          preset.Color,
			PresetCategory: preset.Category,
		}

		et, err := p.entityService.CreateEntityType(ctx, campaignID, input)
		if err != nil {
			slog.Warn("failed to create entity type from preset",
				slog.String("campaign_id", campaignID),
				slog.String("preset", preset.Slug),
				slog.Any("error", err),
			)
			continue // Graceful degradation — skip this preset but try others.
		}

		slog.Info("entity type created from system preset",
			slog.String("campaign_id", campaignID),
			slog.Int("entity_type_id", et.ID),
			slog.String("preset", preset.Slug),
			slog.String("system", systemSlug),
		)
		created++
	}

	return created, nil
}
