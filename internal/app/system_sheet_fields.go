package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// system_sheet_fields.go gives the categories a game system's sheets live on
// the system's own fields, and tells AI Import what the system says about
// them. A system adds its fields to the type its preset matches by name or
// preset category, so a campaign whose heroes live in "Character" and whose
// monsters live in a "Creature" made before the system was enabled never got
// them. Those fields are also how GM-only values are kept from players, so
// they belong on the category, not just in the page data Foundry writes.

// systemSheetFieldsKey is the site setting that records the sweep ran.
const systemSheetFieldsKey = "reconcile.system_sheet_fields_added"

// campaignManifest is the campaign's game system: its custom upload, else its
// chosen built-in; nil when it has none.
type campaignManifest func(ctx context.Context, campaignID string) *systems.SystemManifest

// sheetFieldTypes is the slice of the entity service the sweep needs.
type sheetFieldTypes interface {
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	ReconcileEntityTypeFields(ctx context.Context, typeID int, declared []entities.FieldDefinition) (int, error)
}

// presetTypeIDs maps each of the system's sheet presets to the campaign's
// categories it belongs on: the character preset to the player-character
// list's types (and any type already bound to that preset), the creature
// preset to the creature types.
func presetTypeIDs(ctx context.Context, m *systems.SystemManifest, types []entities.EntityType, lists entities.CharacterListReader, campaignID string) map[*systems.EntityPresetDef][]int {
	out := map[*systems.EntityPresetDef][]int{}
	for i := range m.EntityPresets {
		p := &m.EntityPresets[i]
		var ids []int
		switch p.Category {
		case presetCategoryCharacter:
			seen := map[int]bool{}
			if lists != nil {
				if pcs, err := lists.CharacterTypeIDs(ctx, campaignID); err == nil {
					for _, id := range pcs {
						seen[id] = true
						ids = append(ids, id)
					}
				}
			}
			for _, t := range types {
				if t.PresetCategory != nil && *t.PresetCategory == p.Category && !seen[t.ID] {
					ids = append(ids, t.ID)
				}
			}
		case "creature":
			ids = entities.CreatureTypeIDs(types)
		}
		if len(ids) > 0 {
			out[p] = ids
		}
	}
	return out
}

// addSystemSheetFieldsOnce adds every campaign's system sheet fields to the
// categories they belong on, once per site. It only adds fields a category
// lacks (by key), so a field the owner already has, or renamed, keeps its
// label; afterwards an owner who deletes one keeps it deleted. The setting is
// written only after every campaign was tried, so a crash repeats the sweep,
// which is harmless.
func addSystemSheetFieldsOnce(ctx context.Context, st pageExtrasSettings, camps pageExtrasCampaigns, manifest campaignManifest, ents sheetFieldTypes, lists entities.CharacterListReader) (int, error) {
	if v, err := st.Get(ctx, systemSheetFieldsKey); err == nil && v == "1" {
		return 0, nil
	} else if err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != 404 {
			return 0, fmt.Errorf("reading %s: %w", systemSheetFieldsKey, err)
		}
	}

	added := 0
	for page := 1; ; page++ {
		list, total, err := camps.ListAll(ctx, campaigns.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return added, fmt.Errorf("listing campaigns: %w", err)
		}
		for _, c := range list {
			m := manifest(ctx, c.ID)
			if m == nil {
				continue
			}
			types, err := ents.GetEntityTypes(ctx, c.ID)
			if err != nil {
				slog.Warn("system sheet fields: entity types unavailable",
					slog.String("campaign_id", c.ID), slog.Any("error", err))
				continue
			}
			for p, ids := range presetTypeIDs(ctx, m, types, lists, c.ID) {
				declared := mapPresetFields(p.Fields)
				for _, id := range ids {
					n, err := ents.ReconcileEntityTypeFields(ctx, id, declared)
					if err != nil {
						slog.Warn("system sheet fields: adding failed",
							slog.String("campaign_id", c.ID), slog.Int("entity_type_id", id), slog.Any("error", err))
						continue
					}
					added += n
				}
			}
		}
		if len(list) == 0 || page*100 >= total {
			break
		}
	}
	if err := st.Set(ctx, systemSheetFieldsKey, "1"); err != nil {
		return added, fmt.Errorf("recording %s: %w", systemSheetFieldsKey, err)
	}
	return added, nil
}

// aiFieldNotes tells AI Import's brief what the campaign's game system says
// about a category's fields.
type aiFieldNotes struct {
	manifest campaignManifest
	lists    entities.CharacterListReader
	types    interface {
		GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	}
}

// foundryKeeps reports whether Foundry is where a character field's value
// comes from: it is read off the actor's items, or never written back. A
// value written here would be replaced at the next sync.
func foundryKeeps(f systems.FieldDef) bool {
	if f.FoundryCollection != "" {
		return true
	}
	return f.FoundryPath != "" && f.FoundryWritable != nil && !*f.FoundryWritable
}

// FieldNotes is the system's notes on t's fields by key, or nil.
func (n aiFieldNotes) FieldNotes(ctx context.Context, campaignID string, t entities.EntityType) map[string]string {
	if n.manifest == nil || n.types == nil {
		return nil
	}
	m := n.manifest(ctx, campaignID)
	if m == nil {
		return nil
	}
	types, err := n.types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for p, ids := range presetTypeIDs(ctx, m, types, n.lists, campaignID) {
		for _, id := range ids {
			if id != t.ID {
				continue
			}
			// A category the system's sheet lives on can still carry the
			// starting set's fields from another game (race, alignment), so
			// those are named as not the system's own.
			own := map[string]bool{}
			for _, f := range p.Fields {
				own[f.Key] = true
			}
			system := m.Name
			if system == "" {
				system = "game system"
			}
			for _, d := range t.Fields {
				if !own[d.Key] {
					out[d.Key] = "Not a " + system + " field; fill it only when I ask for it"
				}
			}
			for _, f := range p.Fields {
				note := f.Format
				if p.Category == presetCategoryCharacter && foundryKeeps(f) {
					if note != "" {
						note += ". "
					}
					note += "Foundry keeps this one: leave it out"
				}
				if note != "" {
					out[f.Key] = note
				}
			}
		}
	}
	return out
}
