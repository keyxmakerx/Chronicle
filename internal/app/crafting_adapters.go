package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// craftingDirectoryAdapter gives the crafting plugin the campaign's Armory
// items and characters through the Armory's own directory, so both read
// "item" and "character" the same way and crafting never reads the
// entities tables.
type craftingDirectoryAdapter struct {
	dir *armoryStashDirectoryAdapter
}

func (a *craftingDirectoryAdapter) Entity(ctx context.Context, campaignID, id string) (*crafting.EntityInfo, error) {
	ref, err := a.dir.GetEntity(ctx, campaignID, id)
	if err != nil || ref == nil {
		return nil, err
	}
	// A map handout is an item only so it can be carried; it is never a
	// material or something a recipe makes.
	return &crafting.EntityInfo{ID: ref.ID, Name: ref.Name, IsItem: ref.IsItem && ref.HandoutMapID == "", IsCharacter: ref.IsCharacter}, nil
}

func (a *craftingDirectoryAdapter) ListItems(ctx context.Context, campaignID string, role int, userID string) ([]crafting.Named, bool, error) {
	ti, err := a.dir.types(ctx, campaignID)
	if err != nil {
		return nil, false, err
	}
	refs, truncated, err := a.dir.listByTypes(ctx, ti, campaignID, ti.itemIDs, role, userID, 0)
	if err != nil {
		return nil, false, err
	}
	out := make([]crafting.Named, 0, len(refs))
	for _, r := range refs {
		if r.IsItem && r.HandoutMapID == "" {
			out = append(out, crafting.Named{ID: r.ID, Name: r.Name})
		}
	}
	return out, truncated, nil
}

func (a *craftingDirectoryAdapter) ListCharacters(ctx context.Context, campaignID string, role int, userID string) ([]crafting.Named, bool, error) {
	refs, truncated, err := a.dir.ListCharactersChecked(ctx, campaignID, role, userID)
	if err != nil {
		return nil, false, err
	}
	out := make([]crafting.Named, 0, len(refs))
	for _, r := range refs {
		out = append(out, crafting.Named{ID: r.ID, Name: r.Name})
	}
	return out, truncated, nil
}

func (a *craftingDirectoryAdapter) OwnedCharacterIDs(ctx context.Context, campaignID, userID string) (map[string]bool, error) {
	return a.dir.OwnedCharacterIDs(ctx, campaignID, userID)
}

// craftingRulesAdapter reads the crafting block of the campaign's enabled
// game system; with none enabled the defaults apply.
type craftingRulesAdapter struct {
	sys interface {
		EnabledSystem(ctx context.Context, campaignID string) systems.System
	}
}

func (a *craftingRulesAdapter) CraftingRules(ctx context.Context, campaignID string) (systems.CraftingRules, string) {
	if a.sys == nil {
		return systems.DefaultCraftingRules(), ""
	}
	s := a.sys.EnabledSystem(ctx, campaignID)
	if s == nil || s.Info() == nil {
		return systems.DefaultCraftingRules(), ""
	}
	return s.Info().CraftingRules(), s.Info().Name
}

// craftingCalendarAdapter names the default calendar the rules editor reads.
type craftingCalendarAdapter struct {
	cal headerCalendarService
}

func (a *craftingCalendarAdapter) DefaultCalendarID(ctx context.Context, campaignID string, role int, userID string) string {
	return headerSkyCalendarID(ctx, a.cal, campaignID, permissions.RequestViewer(role, userID))
}
