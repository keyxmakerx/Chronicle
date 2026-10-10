// Package app wires together all application dependencies.
// This file implements the addons.PresetApplier interface, bridging the
// systems package (manifest data) and entities package (entity type creation)
// to auto-create entity types when a game system addon is enabled.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
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
	return p.applyPresets(ctx, campaignID, systemSlug, true, nil)
}

// ReconcileSystemPresets adds the preset fields a campaign's existing entity
// types are missing, and nothing else: it never creates a type (a GM may have
// deleted one on purpose) and never removes, renames or reorders fields.
// known is the previous package version's field keys per preset slug; fields
// in it are skipped so a field a GM deleted stays deleted, and only fields the
// update introduced are added. Returns the number of fields added.
func (p *presetApplier) ReconcileSystemPresets(ctx context.Context, campaignID, systemSlug string, known presetFieldKeys) (int, error) {
	return p.applyPresets(ctx, campaignID, systemSlug, false, known)
}

// presetCategoryCharacter is the system preset category that binds the
// character sheet.
const presetCategoryCharacter = "character"

// presetFieldKeys maps preset slug → set of field keys a package version declares.
type presetFieldKeys map[string]map[string]bool

// snapshotPresetFieldKeys records every installed system's preset field keys
// (system slug → preset slug → keys), taken before a package update replaces
// the loaded manifest.
func snapshotPresetFieldKeys() map[string]presetFieldKeys {
	out := map[string]presetFieldKeys{}
	for _, m := range systems.Registry() {
		if m == nil {
			continue
		}
		keys := presetFieldKeys{}
		for _, preset := range m.EntityPresets {
			set := map[string]bool{}
			for _, f := range preset.Fields {
				set[f.Key] = true
			}
			keys[preset.Slug] = set
		}
		out[m.ID] = keys
	}
	return out
}

// applyPresets is the shared walk behind ApplySystemPresets and
// ReconcileSystemPresets. With create=false a preset with no matching type is
// skipped. A non-nil known drops those previously-declared fields from the
// merge. The count is types created when create is true, fields added otherwise.
func (p *presetApplier) applyPresets(ctx context.Context, campaignID, systemSlug string, create bool, known presetFieldKeys) (int, error) {
	manifest := systems.Find(systemSlug)
	if manifest == nil {
		// System not found in registry — may be a custom upload without
		// bundled manifest. Not an error, just nothing to apply.
		return 0, nil
	}
	return p.applyManifestPresets(ctx, campaignID, systemSlug, manifest, create, known)
}

// applyManifestPresets is applyPresets past the registry lookup, split out so
// the walk can be tested against an in-memory manifest.
func (p *presetApplier) applyManifestPresets(ctx context.Context, campaignID, systemSlug string, manifest *systems.SystemManifest, create bool, known presetFieldKeys) (int, error) {
	if len(manifest.EntityPresets) == 0 {
		return 0, nil
	}

	// Get existing entity types to avoid duplicate creation.
	existingTypes, err := p.entityService.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return 0, fmt.Errorf("listing existing types: %w", err)
	}

	// Index existing types by preset category and by (lowercased) name so each
	// preset can find its already-created type to upgrade in place, or know it
	// must create a new one. Name indexing catches types created before
	// preset_category existed, or made manually by the user.
	existingByCategory := make(map[string]*entities.EntityType)
	existingByName := make(map[string]*entities.EntityType, len(existingTypes))
	for i := range existingTypes {
		et := &existingTypes[i]
		if et.PresetCategory != nil && *et.PresetCategory != "" {
			existingByCategory[*et.PresetCategory] = et
		}
		existingByName[strings.ToLower(et.Name)] = et
	}

	fieldsAdded := 0
	created := 0
	for _, preset := range manifest.EntityPresets {
		declared := mapPresetFields(preset.Fields)
		if known != nil {
			kept := declared[:0:0]
			for _, f := range declared {
				if !known[preset.Slug][f.Key] {
					kept = append(kept, f)
				}
			}
			declared = kept
		}

		// Does a type for this preset already exist? Prefer a preset-category
		// match (stable across renames); fall back to a name match.
		var match *entities.EntityType
		if preset.Category != "" {
			match = existingByCategory[preset.Category]
		}
		if match == nil {
			match = existingByName[strings.ToLower(preset.Name)]
		}

		// Upgrade path: the type exists, so don't recreate it — just add any
		// newly-declared fields it's missing. Idempotent: no-ops once the
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
			fieldsAdded += added
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

		if !create {
			continue
		}

		// A character sheet belongs on the seeded "Characters" type: real
		// characters (and the Foundry module's picks) live there, so a second
		// type would sit empty and the sheet would never render. Only when that
		// type is absent or already bound to another preset do we create one.
		if preset.Category == presetCategoryCharacter {
			adopted, err := p.entityService.AdoptDefaultCharacterType(ctx, campaignID, preset.Category, declared)
			if err != nil {
				slog.Warn("failed to adopt the default Characters type for a system preset",
					slog.String("campaign_id", campaignID),
					slog.String("preset", preset.Slug),
					slog.Any("error", err),
				)
				continue // Don't fall through to creating a duplicate on a transient error.
			}
			if adopted != nil {
				slog.Info("default Characters type adopted as the system character type",
					slog.String("campaign_id", campaignID),
					slog.Int("entity_type_id", adopted.ID),
					slog.String("preset", preset.Slug),
					slog.String("system", systemSlug),
				)
				created++
				continue
			}
		}

		// Create path: no matching type yet — make a new one with its fields.
		// A bad manifest icon falls back to the default rather than dropping
		// the preset.
		icon, replaced := sanitize.IconOrDefault(preset.Icon, "")
		if replaced {
			slog.Warn("system preset has an invalid icon; using the default",
				slog.String("preset", preset.Slug), slog.String("icon", preset.Icon))
		}
		input := entities.CreateEntityTypeInput{
			Name:           preset.Name,
			NamePlural:     preset.NamePlural,
			Icon:           icon,
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

	if !create {
		return fieldsAdded, nil
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
			// Carried onto the stored field def so the egress filter can
			// strip GM secrets from non-GM callers.
			GMOnly: f.GMOnly,
			// Carried so the egress filter can strip player-private content
			// (e.g. backstory) from viewers who aren't the entity's owner.
			OwnerOnly: f.OwnerOnly,
			// Names the data file the pick-from-the-system control reads.
			Choices: f.Choices,
			// Carried so play controls read one stored declaration.
			Play: mapPlayDef(f.Play),
		})
	}
	return out
}

// mapPlayDef copies a manifest play block onto the entities type. Slices are
// copied so the stored field never aliases the registry's manifest.
func mapPlayDef(p *systems.PlayDef) *entities.FieldPlay {
	if p == nil {
		return nil
	}
	out := &entities.FieldPlay{
		Edit:            p.Edit,
		Kind:            p.Kind,
		Min:             p.Min,
		Max:             p.Max,
		MaxField:        p.MaxField,
		Step:            p.Step,
		MaxLength:       p.MaxLength,
		ToFoundry:       p.ToFoundry,
		CombatAuthority: p.CombatAuthority,
	}
	if len(p.Options) > 0 {
		out.Options = append([]string(nil), p.Options...)
	}
	return out
}

// buildPlayByCategory is buildFlagsByCategory's counterpart for play blocks:
// every declared preset field is recorded (nil when it has no block) so the
// reconciler also clears a block a system update removed. Stored entity types
// do not record which system they came from, so when two systems declare the
// same category and key with different blocks neither is written; otherwise
// one system's play rules would land on the other system's characters.
func buildPlayByCategory() map[string]map[string]*entities.FieldPlay {
	return playByCategory(systems.Registry())
}

func playByCategory(manifests []*systems.SystemManifest) map[string]map[string]*entities.FieldPlay {
	out := map[string]map[string]*entities.FieldPlay{}
	conflicted := map[string]map[string]bool{}
	for _, m := range manifests {
		if m == nil {
			continue
		}
		for _, preset := range m.EntityPresets {
			if preset.Category == "" {
				continue
			}
			cat := preset.Category
			for _, f := range preset.Fields {
				if conflicted[cat][f.Key] {
					continue
				}
				if out[cat] == nil {
					out[cat] = map[string]*entities.FieldPlay{}
				}
				want := mapPlayDef(f.Play)
				if prev, seen := out[cat][f.Key]; seen && !reflect.DeepEqual(prev, want) {
					delete(out[cat], f.Key)
					if conflicted[cat] == nil {
						conflicted[cat] = map[string]bool{}
					}
					conflicted[cat][f.Key] = true
					continue
				}
				out[cat][f.Key] = want
			}
		}
	}
	return out
}

// reconcileFieldPlay stamps manifest play blocks onto existing entity types.
// Same idempotent, best-effort contract as reconcileFieldGMFlags.
func reconcileFieldPlay(ctx context.Context, entityService entities.EntityService) {
	play := buildPlayByCategory()
	if len(play) == 0 {
		return
	}
	n, err := entityService.SyncFieldPlay(ctx, play)
	if err != nil {
		slog.Warn("entity_types: play field sync failed", slog.Any("error", err))
		return
	}
	if n > 0 {
		slog.Info("entity_types: play field sync updated types", slog.Int("rows", n))
	}
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

// buildFlagsByCategory reads every installed system manifest and returns a
// (preset-category → field-key → flag) map, where get selects which boolean
// annotation on a manifest field to record (GMOnly or OwnerOnly). Every
// declared field key is recorded (including a false value) so the reconciler
// converges in BOTH directions — a manifest can newly mark a field, or
// un-mark one. On the rare key collision across two systems sharing a preset
// category, the last manifest wins (keys are normally system-specific).
// Nil-safe.
func buildFlagsByCategory(get func(systems.FieldDef) bool) map[string]map[string]bool {
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
				out[cat][f.Key] = get(f)
			}
		}
	}
	return out
}

// buildGMFlagsByCategory is buildFlagsByCategory specialized to the gm_only
// annotation. See buildFlagsByCategory for the convergence rationale.
func buildGMFlagsByCategory() map[string]map[string]bool {
	return buildFlagsByCategory(func(f systems.FieldDef) bool { return f.GMOnly })
}

// buildOwnerOnlyFlagsByCategory is buildFlagsByCategory specialized to the
// owner_only annotation. See buildFlagsByCategory.
func buildOwnerOnlyFlagsByCategory() map[string]map[string]bool {
	return buildFlagsByCategory(func(f systems.FieldDef) bool { return f.OwnerOnly })
}

// reconcileFieldGMFlags stamps the gm_only field flags declared by installed
// system manifests onto existing entity types, making the GM-field egress
// filter effective for characters created before their type's manifest
// carried gm_only. Idempotent; safe at boot and after a system package
// install/update. Best-effort: logs and returns on error rather than
// blocking boot or an install.
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

// reconcileFieldOwnerOnlyFlags is reconcileFieldGMFlags's counterpart for the
// owner_only annotation — same idempotent, best-effort contract, different flag.
func reconcileFieldOwnerOnlyFlags(ctx context.Context, entityService entities.EntityService) {
	flags := buildOwnerOnlyFlagsByCategory()
	if len(flags) == 0 {
		return
	}
	n, err := entityService.SyncFieldOwnerOnlyFlags(ctx, flags)
	if err != nil {
		slog.Warn("entity_types: owner-only-flag field sync failed", slog.Any("error", err))
		return
	}
	if n > 0 {
		slog.Info("entity_types: owner-only-flag field sync updated types", slog.Int("rows", n))
	}
}
