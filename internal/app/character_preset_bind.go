package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// character_preset_bind.go heals campaigns whose game system's character
// preset is named like the seeded Characters type ("Character", as D&D's
// is). Turning such a system on matched Characters by name and only added
// fields, so nothing was bound to the character preset: the hero creator
// then put heroes on the claiming add-on's Player Character category, which
// has no fields. This binds Characters to the preset, as turning the system
// on now does, and gives an empty Player Character category the sheet's
// fields so heroes already made there have them.

// characterPresetBindKey is the site setting that records the sweep ran.
const characterPresetBindKey = "reconcile.character_preset_bound"

// characterPresetTypes is the slice of the entity service the sweep needs.
type characterPresetTypes interface {
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	AdoptDefaultCharacterType(ctx context.Context, campaignID, category string, declared []entities.FieldDefinition) (*entities.EntityType, error)
	ReconcileEntityTypeFields(ctx context.Context, typeID int, declared []entities.FieldDefinition) (int, error)
}

// characterPreset is the manifest's character preset, or nil.
func characterPreset(m *systems.SystemManifest) *systems.EntityPresetDef {
	for i := range m.EntityPresets {
		if m.EntityPresets[i].Category == presetCategoryCharacter {
			return &m.EntityPresets[i]
		}
	}
	return nil
}

// bindCharacterPresetOnce runs the heal once per site. It touches a campaign
// only in the state the name match left: no category bound to the character
// preset, and a top-level Characters type with no preset whose name is the
// preset's. Fields go only onto a Player Character category that has none,
// so a field an owner deleted elsewhere stays deleted. The setting is written
// after every campaign was tried, so a crash repeats the sweep harmlessly.
func bindCharacterPresetOnce(ctx context.Context, st pageExtrasSettings, camps pageExtrasCampaigns, manifest campaignManifest, ents characterPresetTypes) (int, error) {
	if v, err := st.Get(ctx, characterPresetBindKey); err == nil && v == "1" {
		return 0, nil
	} else if err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != 404 {
			return 0, fmt.Errorf("reading %s: %w", characterPresetBindKey, err)
		}
	}

	healed := 0
	for page := 1; ; page++ {
		list, total, err := camps.ListAll(ctx, campaigns.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return healed, fmt.Errorf("listing campaigns: %w", err)
		}
		for _, c := range list {
			if bindCharacterPreset(ctx, c.ID, manifest(ctx, c.ID), ents) {
				healed++
			}
		}
		if len(list) == 0 || page*100 >= total {
			break
		}
	}
	if err := st.Set(ctx, characterPresetBindKey, "1"); err != nil {
		return healed, fmt.Errorf("recording %s: %w", characterPresetBindKey, err)
	}
	return healed, nil
}

// bindCharacterPreset heals one campaign and reports whether it changed.
func bindCharacterPreset(ctx context.Context, campaignID string, m *systems.SystemManifest, ents characterPresetTypes) bool {
	if m == nil {
		return false
	}
	p := characterPreset(m)
	if p == nil {
		return false
	}
	types, err := ents.GetEntityTypes(ctx, campaignID)
	if err != nil {
		slog.Warn("character preset bind: entity types unavailable",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
		return false
	}
	var chars *entities.EntityType
	var empties []int
	for i := range types {
		t := &types[i]
		cat := ""
		if t.PresetCategory != nil {
			cat = *t.PresetCategory
		}
		switch {
		case cat == presetCategoryCharacter:
			return false // already bound
		case t.ParentTypeID == nil && t.Slug == entities.DefaultCharacterTypeSlug && cat == "":
			chars = t
		case cat == entities.PresetCategoryPlayerCharacter && len(t.Fields) == 0:
			empties = append(empties, t.ID)
		}
	}
	if chars == nil || !strings.EqualFold(chars.Name, p.Name) {
		return false
	}
	// Characters already has the preset's fields from the name match; none
	// are passed, so one the owner deleted there stays deleted.
	adopted, err := ents.AdoptDefaultCharacterType(ctx, campaignID, presetCategoryCharacter, nil)
	if err != nil || adopted == nil {
		if err != nil {
			slog.Warn("character preset bind: binding failed",
				slog.String("campaign_id", campaignID), slog.Any("error", err))
		}
		return false
	}
	declared := mapPresetFields(p.Fields)
	for _, id := range empties {
		if _, err := ents.ReconcileEntityTypeFields(ctx, id, declared); err != nil {
			slog.Warn("character preset bind: adding fields failed",
				slog.String("campaign_id", campaignID), slog.Int("entity_type_id", id), slog.Any("error", err))
		}
	}
	return true
}
