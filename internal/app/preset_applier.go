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

	// Index existing types by preset category and by (lowercased) name so each
	// preset can find its already-created type — either to upgrade it in place
	// (WS-5) or to know it must create a new one. Name indexing catches types
	// created before preset_category existed, or made manually by the user.
	existingByCategory := make(map[string]*entities.EntityType)
	existingByName := make(map[string]*entities.EntityType, len(existingTypes))
	for i := range existingTypes {
		et := &existingTypes[i]
		if et.PresetCategory != nil && *et.PresetCategory != "" {
			existingByCategory[*et.PresetCategory] = et
		}
		existingByName[strings.ToLower(et.Name)] = et
	}

	created := 0
	for _, preset := range manifest.EntityPresets {
		declared := mapPresetFields(preset.Fields)

		// Does a type for this preset already exist? Prefer a preset-category
		// match (stable across renames); fall back to a name match.
		var match *entities.EntityType
		if preset.Category != "" {
			match = existingByCategory[preset.Category]
		}
		if match == nil {
			match = existingByName[strings.ToLower(preset.Name)]
		}

		// Upgrade path (WS-5): the type exists, so don't recreate it — just add
		// any newly-declared fields it's missing. Idempotent: no-ops once the
		// type already carries every declared field.
		if match != nil {
			added, err := p.entityService.ReconcileEntityTypeFields(ctx, match.ID, declared)
			if err != nil {
				slog.Warn("failed to reconcile entity type fields from preset",
					slog.String("campaign_id", campaignID),
					slog.String("preset", preset.Slug),
					slog.Int("entity_type_id", match.ID),
					slog.Any("error", err),
				)
				continue // Graceful degradation — try the other presets.
			}
			if added > 0 {
				slog.Info("entity type fields upgraded from system preset",
					slog.String("campaign_id", campaignID),
					slog.Int("entity_type_id", match.ID),
					slog.String("preset", preset.Slug),
					slog.String("system", systemSlug),
					slog.Int("fields_added", added),
				)
			}
			continue
		}

		// Create path: no matching type yet — make a new one with its fields.
		input := entities.CreateEntityTypeInput{
			Name:           preset.Name,
			NamePlural:     preset.NamePlural,
			Icon:           preset.Icon,
			Color:          preset.Color,
			PresetCategory: preset.Category,
			Fields:         declared,
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

// ApplyAddonEnableEffects runs entity-type side effects for non-system addons on
// enable. Today only the Player Character Claiming addon has one: premaking the
// claimable "Player Characters" type (idempotent in the service).
func (p *presetApplier) ApplyAddonEnableEffects(ctx context.Context, campaignID, addonSlug string) error {
	switch addonSlug {
	case entities.AddonPlayerCharacterClaiming:
		return p.entityService.EnsurePlayerCharacterType(ctx, campaignID)
	}
	return nil
}

// mapPresetFields converts a system manifest's preset field definitions into the
// entity-type field schema Chronicle stores. The manifest's Foundry-sync
// annotations (foundry_path, foundry_collection, …) are intentionally NOT copied
// here — those are served separately to the Foundry module via the character-fields
// API; the entity type only needs the display schema. Returns nil for no fields
// (the service normalizes nil → []).
func mapPresetFields(fields []systems.FieldDef) []entities.FieldDefinition {
	if len(fields) == 0 {
		return nil
	}
	out := make([]entities.FieldDefinition, 0, len(fields))
	for _, f := range fields {
		out = append(out, entities.FieldDefinition{
			Key:   f.Key,
			Label: f.Label,
			Type:  mapPresetFieldType(f.Type),
			// Carry the GM-only marker onto the stored field def so the
			// egress filter can strip GM secrets for non-GM callers
			// (C-FIELDS-GM-FILTER / M-1).
			GMOnly: f.GMOnly,
		})
	}
	return out
}

// mapPresetFieldType maps a manifest field type ("string", "number", "boolean",
// "list", "markdown", "enum", "url") onto the entity-form input types Chronicle
// renders ("text", "number", "checkbox", "textarea", "select", "url"). Unknown
// types fall back to "text".
func mapPresetFieldType(t string) string {
	switch t {
	case "number":
		return "number"
	case "boolean":
		return "checkbox"
	case "enum":
		return "select"
	case "markdown", "list":
		return "textarea"
	case "url":
		return "url"
	default: // "string" and anything unrecognized
		return "text"
	}
}

// buildGMFlagsByCategory reads every installed system manifest and returns a
// (preset-category → field-key → gm_only) map of the declared GM-only field
// flags. Every declared field key is recorded (including gm_only=false) so
// the reconciler converges in BOTH directions — a manifest can newly mark a
// field gm_only, or un-mark one. On the rare key collision across two systems
// sharing a preset category, the last manifest wins (keys are normally
// system-specific). Nil-safe.
func buildGMFlagsByCategory() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, m := range systems.Registry() {
		if m == nil {
			continue
		}
		for _, preset := range m.EntityPresets {
			cat := preset.Category
			if cat == "" {
				continue
			}
			for _, f := range preset.Fields {
				if out[cat] == nil {
					out[cat] = map[string]bool{}
				}
				out[cat][f.Key] = f.GMOnly
			}
		}
	}
	return out
}

// reconcileFieldGMFlags stamps the gm_only field flags declared by installed
// system manifests onto existing entity types (audit M-1 convergence). This
// is what makes the GM-field egress filter effective for characters created
// BEFORE the system's manifest carried gm_only — the flag isn't on their
// stored field defs until this runs. Idempotent; safe at boot and after a
// system package install/update. Best-effort: logs and returns on error so a
// reconcile hiccup never blocks boot or an install.
func reconcileFieldGMFlags(ctx context.Context, entityService entities.EntityService) {
	flags := buildGMFlagsByCategory()
	if len(flags) == 0 {
		return
	}
	n, err := entityService.SyncFieldGMFlags(ctx, flags)
	if err != nil {
		slog.Warn("entity_types: gm-flag field sync failed", slog.Any("error", err))
		return
	}
	if n > 0 {
		slog.Info("entity_types: gm-flag field sync updated types", slog.Int("rows", n))
	}
}
