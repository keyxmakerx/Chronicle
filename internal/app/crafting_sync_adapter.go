package app

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

// syncCraftingAPIAdapter implements syncapi.CraftingAPIService over the
// crafting service, acting as the member the call names. Who the call runs
// as is decided by StashAPI.ActorFor, the rule the stash and shop calls use,
// so a key acts only as itself or, when its holder is Owner or co-DM, as a
// current member under that member's own rights. Crafting then applies the
// same checks as the web routes: a player works only on characters they
// claimed, and the tray and fights are the Director's.
//
// The service attaches once the crafting plugin is built; until then, and
// while crafting is degraded, every call is NotFound.
type syncCraftingAPIAdapter struct {
	actors *armory.StashAPI
	svc    crafting.Service
}

var _ syncapi.CraftingAPIService = (*syncCraftingAPIAdapter)(nil)

func (a *syncCraftingAPIAdapter) attach(svc crafting.Service) { a.svc = svc }

// viewer resolves the acting member as crafting's Viewer. The actor's role is
// already promoted to Owner by a co-DM grant, which is crafting's Director,
// as on the web.
func (a *syncCraftingAPIAdapter) viewer(ctx context.Context, campaignID, keyUserID, actingUserID string) (crafting.Viewer, error) {
	if a.svc == nil {
		return crafting.Viewer{}, apperror.NewNotFound("crafting is not available")
	}
	actor, err := a.actors.ActorFor(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return crafting.Viewer{}, err
	}
	return crafting.Viewer{UserID: actor.UserID, MemberRole: actor.Role, VisibilityRole: actor.Role,
		Director: actor.Role >= permissions.RoleOwner}, nil
}

// decodeCrafting reads a body into crafting's own input type. Unknown fields,
// actingUserId among them, are ignored.
func decodeCrafting(body json.RawMessage, dst any) error {
	if err := json.Unmarshal(body, dst); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	return nil
}

func (a *syncCraftingAPIAdapter) Character(ctx context.Context, campaignID, keyUserID, actingUserID, characterID string) (any, error) {
	v, err := a.viewer(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	return a.svc.Character(ctx, campaignID, characterID, v)
}

func (a *syncCraftingAPIAdapter) StartProject(ctx context.Context, campaignID, keyUserID, actingUserID, characterID string, body json.RawMessage) (any, error) {
	v, err := a.viewer(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	var in crafting.StartInput
	if err := decodeCrafting(body, &in); err != nil {
		return nil, err
	}
	return a.svc.StartProject(ctx, campaignID, characterID, v, in)
}

func (a *syncCraftingAPIAdapter) RollProject(ctx context.Context, campaignID, keyUserID, actingUserID, projectID string, body json.RawMessage) (any, error) {
	v, err := a.viewer(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	var in crafting.RollInput
	if err := decodeCrafting(body, &in); err != nil {
		return nil, err
	}
	return a.svc.RollProject(ctx, campaignID, projectID, v, in)
}

func (a *syncCraftingAPIAdapter) CancelProject(ctx context.Context, campaignID, keyUserID, actingUserID, projectID string) error {
	v, err := a.viewer(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return err
	}
	return a.svc.CancelProject(ctx, campaignID, projectID, v)
}

func (a *syncCraftingAPIAdapter) Spoils(ctx context.Context, campaignID, keyUserID, actingUserID string) (any, error) {
	v, err := a.viewer(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	return a.svc.Spoils(ctx, campaignID, v)
}

func (a *syncCraftingAPIAdapter) TakeSpoil(ctx context.Context, campaignID, keyUserID, actingUserID string, lineID int64, body json.RawMessage) (any, error) {
	v, err := a.viewer(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	var in crafting.TakeInput
	if err := decodeCrafting(body, &in); err != nil {
		return nil, err
	}
	return a.svc.TakeSpoil(ctx, campaignID, lineID, v, in)
}

func (a *syncCraftingAPIAdapter) StudySpoil(ctx context.Context, campaignID, keyUserID, actingUserID string, lineID int64, body json.RawMessage) (any, error) {
	v, err := a.viewer(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	var in crafting.TakeInput
	if err := decodeCrafting(body, &in); err != nil {
		return nil, err
	}
	return a.svc.StudySpoil(ctx, campaignID, lineID, v, in)
}

func (a *syncCraftingAPIAdapter) Experiment(ctx context.Context, campaignID, keyUserID, actingUserID, characterID string, body json.RawMessage) (any, error) {
	v, err := a.viewer(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	var in crafting.TryInput
	if err := decodeCrafting(body, &in); err != nil {
		return nil, err
	}
	return a.svc.Experiment(ctx, campaignID, characterID, v, in)
}

func (a *syncCraftingAPIAdapter) BreakDown(ctx context.Context, campaignID, keyUserID, actingUserID, characterID string, body json.RawMessage) (any, error) {
	v, err := a.viewer(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	var in crafting.BreakInput
	if err := decodeCrafting(body, &in); err != nil {
		return nil, err
	}
	return a.svc.BreakDown(ctx, campaignID, characterID, v, in)
}

func (a *syncCraftingAPIAdapter) Help(ctx context.Context, campaignID, keyUserID, actingUserID, projectID string, body json.RawMessage) (any, error) {
	v, err := a.viewer(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	var in crafting.HelpInput
	if err := decodeCrafting(body, &in); err != nil {
		return nil, err
	}
	return a.svc.Help(ctx, campaignID, projectID, v, in)
}

// syncFight is a fight as the Foundry module reports it once the GM has
// confirmed what dropped: each creature row with a Chronicle page, and how
// many of its copies were down.
type syncFight struct {
	Name      string `json:"name"`
	Creatures []struct {
		PageID string `json:"pageId"`
		Name   string `json:"name"`
		Down   int    `json:"down"`
		Copies int    `json:"copies"`
	} `json:"creatures"`
}

// syncFightSpoils is what the fight left, a line per creature row, for the
// GM's whisper.
type syncFightSpoils struct {
	Lines  []string `json:"lines"`
	Spoils int      `json:"spoils"`
}

func (a *syncCraftingAPIAdapter) FightEnded(ctx context.Context, campaignID, keyUserID, actingUserID string, body json.RawMessage) (any, error) {
	v, err := a.viewer(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	var in syncFight
	if err := decodeCrafting(body, &in); err != nil {
		return nil, err
	}
	fight := crafting.Fight{Name: in.Name, Creatures: make([]crafting.FightCreature, 0, len(in.Creatures))}
	for _, c := range in.Creatures {
		fight.Creatures = append(fight.Creatures, crafting.FightCreature{PageID: c.PageID, Name: c.Name, Down: c.Down, Copies: c.Copies})
	}
	got, err := a.svc.FightEnded(ctx, campaignID, v, fight)
	if err != nil {
		return nil, err
	}
	return syncFightSpoils{Lines: got.Lines, Spoils: got.Spoils}, nil
}
