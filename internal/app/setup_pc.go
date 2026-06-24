// Package app wires together all application dependencies.
// This file implements the addons.SetupProvider for the Player Character
// Claiming addon — the first concrete extension settings/onboarding provider.
// It bridges the addons framework (which owns the settings page + registry) and
// the entities package (which owns the player-character category logic), so the
// addons package never imports entities (mirrors preset_applier.go).
package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// pcSetupProvider implements addons.SetupProvider for the player-character
// claiming addon: it detects existing player-character entities / sub-categories
// / the duplicate-category artifact, lets the owner choose the system vs a custom
// name, and reconciles the duplicate on demand.
type pcSetupProvider struct {
	entityService entities.EntityService
}

// newPCSetupProvider constructs the provider over the entities service.
func newPCSetupProvider(entityService entities.EntityService) *pcSetupProvider {
	return &pcSetupProvider{entityService: entityService}
}

// Slug ties this provider to the player-character claiming addon.
func (p *pcSetupProvider) Slug() string { return entities.AddonPlayerCharacterClaiming }

// RunChecks inspects the campaign's player-character category state.
func (p *pcSetupProvider) RunChecks(ctx context.Context, campaignID string) ([]addons.SetupCheck, error) {
	snap, err := p.entityService.PlayerCharacterSetupSnapshot(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	var checks []addons.SetupCheck

	totalPCs := snap.GenericPCCount + snap.SystemCharCount
	checks = append(checks, addons.SetupCheck{
		ID:       "pc.entities",
		Severity: addons.SeverityInfo,
		Title:    fmt.Sprintf("%d player character%s in this campaign", totalPCs, plural(totalPCs)),
		Detail:   "These are the characters your players can claim.",
	})

	checks = append(checks, addons.SetupCheck{
		ID:       "pc.subcategories",
		Severity: addons.SeverityInfo,
		Title:    fmt.Sprintf("%d sub-categor%s under \"Characters\"", snap.SubCategoryCount, pluralISES(snap.SubCategoryCount)),
		Detail:   "The player-character category lives under your default \"Characters\" category.",
	})

	dup := len(snap.GenericPCTypes) == 1 && len(snap.SystemCharTypes) == 1
	if dup {
		genericName := snap.GenericPCTypes[0].Name
		systemName := snap.SystemCharTypes[0].Name
		checks = append(checks, addons.SetupCheck{
			ID:       "pc.duplicate",
			Severity: addons.SeverityWarning,
			Title:    "Duplicate player-character categories found",
			Detail: fmt.Sprintf("A generic \"%s\" (%d character%s) and your system's \"%s\" both exist. Merge them so claims and the system character sheet line up.",
				genericName, snap.GenericPCCount, plural(snap.GenericPCCount), systemName),
			ActionLabel: fmt.Sprintf("Merge into \"%s\" below", systemName),
		})
	}

	if name := pcTargetName(snap); name != "" {
		checks = append(checks, addons.SetupCheck{
			ID:       "pc.naming",
			Severity: addons.SeveritySuggestion,
			Title:    fmt.Sprintf("Player-character category name: \"%s\"", name),
			Detail:   "Keep your game system's term, or rename it to whatever your table prefers.",
		})
	}

	if snap.DefaultCharsParentID == nil && (len(snap.GenericPCTypes) > 0 || len(snap.SystemCharTypes) > 0) {
		checks = append(checks, addons.SetupCheck{
			ID:       "pc.missing_parent",
			Severity: addons.SeverityWarning,
			Title:    "No default \"Characters\" category",
			Detail:   "Your player-character type isn't nested under a \"Characters\" category. Applying setup will try to organize it.",
		})
	}

	return checks, nil
}

// Questions describes the onboarding questions (naming + optional merge).
func (p *pcSetupProvider) Questions(ctx context.Context, campaignID string) ([]addons.SetupQuestion, error) {
	snap, err := p.entityService.PlayerCharacterSetupSnapshot(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	var qs []addons.SetupQuestion

	if targetName := pcTargetName(snap); targetName != "" {
		qs = append(qs, addons.SetupQuestion{
			ID:            "naming",
			Kind:          addons.QuestionChoice,
			Title:         "Player-character category name",
			Help:          "Keep the current name (your game system's term, if it provides one) or set your own.",
			ShowIfCheckID: "pc.naming",
			Default:       "system",
			Options: []addons.SetupOption{
				{Value: "system", Label: fmt.Sprintf("Keep \"%s\"", targetName)},
				{Value: "custom", Label: "Use a custom name…", Hint: "Enter it below"},
			},
		})
		qs = append(qs, addons.SetupQuestion{
			ID:            "naming_custom",
			Kind:          addons.QuestionText,
			Title:         "Custom category name",
			Help:          "Used only when \"Use a custom name\" is selected.",
			ShowIfCheckID: "pc.naming",
		})
	}

	if len(snap.GenericPCTypes) == 1 && len(snap.SystemCharTypes) == 1 {
		systemName := snap.SystemCharTypes[0].Name
		qs = append(qs, addons.SetupQuestion{
			ID:    "merge",
			Kind:  addons.QuestionBool,
			Title: fmt.Sprintf("Merge the duplicate into \"%s\"", systemName),
			Help: fmt.Sprintf("Moves %d character%s onto \"%s\" and removes the empty generic category.",
				snap.GenericPCCount, plural(snap.GenericPCCount), systemName),
			ShowIfCheckID: "pc.duplicate",
			Default:       "true",
		})
	}

	return qs, nil
}

// Apply runs the safe ensure, then the optional merge, then the optional rename.
// Idempotent — re-applying the same answers is a no-op.
func (p *pcSetupProvider) Apply(ctx context.Context, campaignID string, answers map[string]string) (addons.SetupResult, error) {
	var messages []string

	// 1. Always ensure the PC category exists and is nested (safe, idempotent).
	if err := p.entityService.EnsurePlayerCharacterType(ctx, campaignID); err != nil {
		return addons.SetupResult{}, err
	}

	// 2. Merge the duplicate, if requested.
	if answers["merge"] == "true" {
		res, err := p.entityService.MergeDuplicatePlayerCharacterType(ctx, campaignID)
		if err != nil {
			return addons.SetupResult{}, err
		}
		if !res.NoOp && res.RemovedTypeID != 0 {
			if res.Moved > 0 {
				messages = append(messages, fmt.Sprintf("Moved %d character%s into \"%s\".", res.Moved, plural(res.Moved), res.TargetName))
			}
			messages = append(messages, "Removed the empty duplicate category.")
		}
	}

	// 3. Rename to a custom name, if chosen.
	if answers["naming"] == "custom" {
		if custom := strings.TrimSpace(answers["naming_custom"]); custom != "" {
			renamed, err := p.renamePCCategory(ctx, campaignID, custom)
			if err != nil {
				return addons.SetupResult{}, err
			}
			if renamed != "" {
				messages = append(messages, fmt.Sprintf("Renamed the player-character category to \"%s\".", renamed))
			}
		}
	}

	if len(messages) == 0 {
		messages = append(messages, "Player-character setup is complete.")
	}
	return addons.SetupResult{Messages: messages, Completed: true}, nil
}

// renamePCCategory renames the surviving player-character category. Re-reads the
// snapshot (so it picks up the post-merge survivor) and no-ops if already named.
func (p *pcSetupProvider) renamePCCategory(ctx context.Context, campaignID, custom string) (string, error) {
	snap, err := p.entityService.PlayerCharacterSetupSnapshot(ctx, campaignID)
	if err != nil {
		return "", err
	}
	target := pcSurvivingType(snap)
	if target == nil {
		return "", nil // nothing to rename
	}
	if target.Name == custom {
		return custom, nil // already named; idempotent
	}
	if _, err := p.entityService.UpdateEntityType(ctx, target.ID, entities.UpdateEntityTypeInput{
		Name:         custom,
		NamePlural:   custom,
		Icon:         target.Icon,
		Color:        target.Color,
		ParentTypeID: target.ParentTypeID, // nil = no change (preserve nesting)
	}); err != nil {
		return "", err
	}
	return custom, nil
}

// pcSurvivingType returns the single player-character category that survives
// setup: the system's own character type if present, else the single generic.
func pcSurvivingType(snap entities.PCSetupSnapshot) *entities.EntityType {
	if len(snap.SystemCharTypes) == 1 {
		t := snap.SystemCharTypes[0]
		return &t
	}
	if len(snap.GenericPCTypes) == 1 {
		t := snap.GenericPCTypes[0]
		return &t
	}
	return nil
}

// pcTargetName is the current display name of the surviving PC category, or "".
func pcTargetName(snap entities.PCSetupSnapshot) string {
	if t := pcSurvivingType(snap); t != nil {
		return t.Name
	}
	return ""
}

// plural returns "s" for non-1 counts.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// pluralISES returns the "y"->"ies" / singular suffix for "categor(y|ies)".
func pluralISES(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
