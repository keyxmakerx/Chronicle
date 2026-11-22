package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/changesource"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// aiPlayerSheetEntities is the part of the entities service a player's AI
// changes need.
type aiPlayerSheetEntities interface {
	GetByID(ctx context.Context, id string) (*entities.Entity, error)
	ListClaimed(ctx context.Context, campaignID string, role int, userID string) ([]entities.Entity, error)
	GetEntityTypeByID(ctx context.Context, id int) (*entities.EntityType, error)
	CheckEntityAccess(ctx context.Context, entityID string, role int, userID string) (*entities.EffectivePermission, error)
	MergeFields(ctx context.Context, entityID string, patch map[string]any) error
}

// aiPlayerSheets lets a player's AI change that player's own character, and
// nothing more. Every write passes the checks the sheet's editor makes for
// a claimed owner (entities.CheckClaimedOwnerPatch, judged as a Player
// whatever the person's role, with the hero builder plan that player sees),
// and the page must still be one the player can open.
type aiPlayerSheets struct {
	entities aiPlayerSheetEntities
	planner  entities.HeroPlanner
	// gmTerm is the campaign's word for its game master; nil says "GM".
	gmTerm func(ctx context.Context, campaignID string) string
	// heroChanges locks picks outside the Director's times and rings their
	// bell for a change; nil never locks.
	heroChanges *entities.HeroChangeService
}

var _ ai_workspace.PlayerSheets = aiPlayerSheets{}

func (a aiPlayerSheets) GMTerm(ctx context.Context, campaignID string) string {
	if a.gmTerm == nil {
		return ""
	}
	return a.gmTerm(ctx, campaignID)
}

func (a aiPlayerSheets) Sheets(ctx context.Context, campaignID, userID string) ([]ai_workspace.PlayerSheet, error) {
	claimed, err := a.entities.ListClaimed(ctx, campaignID, int(campaigns.RolePlayer), userID)
	if err != nil {
		return nil, err
	}
	plan, err := a.plan(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	var out []ai_workspace.PlayerSheet
	for i := range claimed {
		e := &claimed[i]
		if !e.IsOwnedBy(userID) || e.CampaignID != campaignID {
			continue
		}
		if ok, err := a.canView(ctx, e.ID, userID); err != nil {
			return nil, err
		} else if !ok {
			continue
		}
		typeFields, typeName, err := a.typeFields(ctx, e)
		if err != nil {
			return nil, err
		}
		if e.TypeName != "" {
			typeName = e.TypeName
		}
		out = append(out, playerSheetOf(e, typeName, typeFields, plan))
	}
	return out, nil
}

// playerSheetOf is one character as the player's AI sees it: each field of
// its category the player may change, once, by the door's own rule, and the
// rest of what the player can see on it.
func playerSheetOf(e *entities.Entity, typeName string, typeFields []entities.FieldDefinition, plan *entities.HeroPlan) ai_workspace.PlayerSheet {
	s := ai_workspace.PlayerSheet{ID: e.ID, Name: e.Name, Type: typeName, Values: map[string]string{},
		Picks: map[string][]string{}, Own: map[string]bool{}, Ask: map[string][]string{},
		Required: map[string]bool{}, Fixed: map[string]string{}, Buys: map[string]ai_workspace.PlayerBuy{}, Shown: map[string]string{}}
	steps := map[string]*entities.HeroStep{}
	if plan != nil {
		for i := range plan.Steps {
			steps[plan.Steps[i].FieldKey] = &plan.Steps[i]
		}
	}
	editable := map[string]bool{}
	for _, f := range typeFields {
		kind, own := entities.OwnerFieldKindOf(f.Key, typeFields, plan)
		if kind == "" || editable[f.Key] {
			continue
		}
		editable[f.Key] = true
		s.Values[f.Key] = fieldText(e.FieldsData[f.Key])
		switch kind {
		case entities.OwnerFieldChoice:
			st := steps[f.Key]
			names := make([]string, 0, len(st.Choices))
			var ask []string
			for _, ch := range st.Choices {
				if ch.State == entities.HeroChoiceStateAskFirst {
					ask = append(ask, ch.Name)
					continue
				}
				names = append(names, ch.Name)
			}
			sort.Strings(names)
			sort.Strings(ask)
			s.Picks[f.Key] = names
			s.Own[f.Key] = own
			if len(ask) > 0 {
				s.Ask[f.Key] = ask
			}
			if st.Rules != nil {
				s.Required[f.Key] = st.Rules.Required
				s.Fixed[f.Key] = st.Rules.Prefilled
			}
			// What the hero's current entry lets them buy, if anything.
			cur := strings.ToLower(strings.TrimSpace(s.Values[f.Key]))
			for _, ch := range st.Choices {
				if strings.ToLower(strings.TrimSpace(ch.Name)) != cur {
					continue
				}
				if names, costs, budget, ok := entities.HeroBuyList(ch); ok {
					k := entities.HeroOptionsKey(f.Key)
					s.Buys[k] = ai_workspace.PlayerBuy{Entry: ch.Name, Budget: budget, Names: names, Costs: costs}
					s.Values[k] = fieldText(e.FieldsData[k])
					editable[k] = true
				}
			}
		case entities.OwnerFieldText:
			s.Free = append(s.Free, f.Key)
		case entities.OwnerFieldStory:
			s.Story = append(s.Story, f.Key)
		case entities.OwnerFieldList:
			s.Lists = append(s.Lists, f.Key)
		}
	}
	// The player owns the character, so their owner-only fields are theirs
	// to read; GM-only ones never reach the AI.
	seen := entities.FilterRestrictedFields(e.FieldsData, typeFields, false, true)
	for _, f := range typeFields {
		if editable[f.Key] || f.GMOnly {
			continue
		}
		if v := strings.TrimSpace(fieldText(seen[f.Key])); v != "" {
			s.Shown[f.Key] = v
		}
	}
	return s
}

func (a aiPlayerSheets) HeldFields(ctx context.Context, campaignID, userID, entityID string, patch map[string]any) (map[string]string, error) {
	e, typeFields, plan, err := a.ownCharacter(ctx, campaignID, userID, entityID)
	if err != nil {
		return nil, err
	}
	held, err := entities.ClaimedOwnerPatchHeld(e, userID, campaignID, patch, typeFields, plan)
	if err != nil {
		return nil, err
	}
	locked, _, err := a.heroChanges.HeldPicks(ctx, e, unheld(patch, held), plan, typeFields)
	if err != nil {
		return nil, err
	}
	for k, why := range locked {
		held[k] = why
	}
	return held, nil
}

// unheld is patch without the keys already held.
func unheld(patch map[string]any, held map[string]string) map[string]any {
	out := make(map[string]any, len(patch))
	for k, v := range patch {
		if _, ok := held[k]; !ok {
			out[k] = v
		}
	}
	return out
}

// ownCharacter loads a character the player may change through their AI:
// one of this campaign they claimed and can still open, with its fields and
// the hero builder plan they see.
func (a aiPlayerSheets) ownCharacter(ctx context.Context, campaignID, userID, entityID string) (*entities.Entity, []entities.FieldDefinition, *entities.HeroPlan, error) {
	e, err := a.entities.GetByID(ctx, entityID)
	if err != nil {
		return nil, nil, nil, err
	}
	if e.CampaignID != campaignID {
		return nil, nil, nil, apperror.NewNotFound("that character isn't in this campaign")
	}
	// A GM can hide a claimed character from its player; the claim alone
	// must not outlive that.
	if ok, err := a.canView(ctx, e.ID, userID); err != nil {
		return nil, nil, nil, err
	} else if !ok {
		return nil, nil, nil, apperror.NewNotFound("that character isn't one you can open")
	}
	typeFields, _, err := a.typeFields(ctx, e)
	if err != nil {
		return nil, nil, nil, err
	}
	plan, err := a.plan(ctx, campaignID, userID)
	if err != nil {
		return nil, nil, nil, err
	}
	return e, typeFields, plan, nil
}

func (a aiPlayerSheets) PatchSheet(ctx context.Context, campaignID, userID, entityID string, patch map[string]any, label string, apply bool) error {
	e, typeFields, plan, err := a.ownCharacter(ctx, campaignID, userID, entityID)
	if err != nil {
		return err
	}
	if err := entities.CheckClaimedOwnerPatch(e, userID, campaignID, patch, typeFields, plan); err != nil {
		return err
	}
	locked, changes, err := a.heroChanges.HeldPicks(ctx, e, patch, plan, typeFields)
	if err != nil {
		return err
	}
	for _, why := range locked {
		return apperror.NewForbidden(why)
	}
	if !apply {
		return nil
	}
	wctx := changesource.With(ctx, changesource.Source{Kind: changesource.KindAI, Label: label, UserID: userID})
	entities.ClearStaleStepOptions(e, patch)
	if err := a.entities.MergeFields(wctx, e.ID, patch); err != nil {
		return err
	}
	a.heroChanges.Changed(ctx, e, userID, changes)
	return nil
}

func (a aiPlayerSheets) plan(ctx context.Context, campaignID, userID string) (*entities.HeroPlan, error) {
	if a.planner == nil {
		return nil, nil
	}
	return a.planner.HeroPlan(ctx, campaignID, userID, false)
}

func (a aiPlayerSheets) canView(ctx context.Context, entityID, userID string) (bool, error) {
	access, err := a.entities.CheckEntityAccess(ctx, entityID, int(campaigns.RolePlayer), userID)
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return access != nil && access.CanView, nil
}

// typeFields is e's category's fields with any its page added, and the
// category's name.
func (a aiPlayerSheets) typeFields(ctx context.Context, e *entities.Entity) ([]entities.FieldDefinition, string, error) {
	et, err := a.entities.GetEntityTypeByID(ctx, e.EntityTypeID)
	if err != nil {
		return nil, "", err
	}
	out := append([]entities.FieldDefinition{}, et.Fields...)
	if e.FieldOverrides != nil {
		out = append(out, e.FieldOverrides.Added...)
	}
	return out, et.Name, nil
}

// fieldText is a sheet value as the prompt and review show it.
func fieldText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}
