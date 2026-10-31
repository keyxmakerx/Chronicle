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
}

var _ ai_workspace.PlayerSheets = aiPlayerSheets{}

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
		gmOnly, typeName, err := a.gmOnly(ctx, e)
		if err != nil {
			return nil, err
		}
		if e.TypeName != "" {
			typeName = e.TypeName
		}
		s := ai_workspace.PlayerSheet{ID: e.ID, Name: e.Name, Type: typeName, Values: map[string]string{}, Picks: map[string][]string{}}
		for _, k := range entities.OwnerIdentityFieldKeys() {
			if !gmOnly[k] {
				s.Free = append(s.Free, k)
				s.Values[k] = fieldText(e.FieldsData[k])
			}
		}
		if plan != nil {
			for _, st := range plan.Steps {
				if gmOnly[st.FieldKey] || len(st.Choices) == 0 {
					continue
				}
				names := make([]string, 0, len(st.Choices))
				for _, ch := range st.Choices {
					names = append(names, ch.Name)
				}
				sort.Strings(names)
				s.Picks[st.FieldKey] = names
				s.Values[st.FieldKey] = fieldText(e.FieldsData[st.FieldKey])
			}
		}
		out = append(out, s)
	}
	return out, nil
}

func (a aiPlayerSheets) PatchSheet(ctx context.Context, campaignID, userID, entityID string, patch map[string]any, label string, apply bool) error {
	e, err := a.entities.GetByID(ctx, entityID)
	if err != nil {
		return err
	}
	if e.CampaignID != campaignID {
		return apperror.NewNotFound("that character isn't in this campaign")
	}
	et, err := a.entities.GetEntityTypeByID(ctx, e.EntityTypeID)
	if err != nil {
		return err
	}
	typeFields := append([]entities.FieldDefinition{}, et.Fields...)
	if e.FieldOverrides != nil {
		typeFields = append(typeFields, e.FieldOverrides.Added...)
	}
	plan, err := a.plan(ctx, campaignID, userID)
	if err != nil {
		return err
	}
	if err := entities.CheckClaimedOwnerPatch(e, userID, campaignID, patch, typeFields, plan); err != nil {
		return err
	}
	// A GM can hide a claimed character from its player; the claim alone
	// must not outlive that.
	if ok, err := a.canView(ctx, e.ID, userID); err != nil {
		return err
	} else if !ok {
		return apperror.NewNotFound("that character isn't one you can open")
	}
	if !apply {
		return nil
	}
	wctx := changesource.With(ctx, changesource.Source{Kind: changesource.KindAI, Label: label, UserID: userID})
	return a.entities.MergeFields(wctx, e.ID, patch)
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

// gmOnly is the keys of e's sheet its player may not see, and its
// category's name.
func (a aiPlayerSheets) gmOnly(ctx context.Context, e *entities.Entity) (map[string]bool, string, error) {
	et, err := a.entities.GetEntityTypeByID(ctx, e.EntityTypeID)
	if err != nil {
		return nil, "", err
	}
	out := map[string]bool{}
	for _, f := range et.Fields {
		if f.GMOnly {
			out[f.Key] = true
		}
	}
	if e.FieldOverrides != nil {
		for _, f := range e.FieldOverrides.Added {
			if f.GMOnly {
				out[f.Key] = true
			}
		}
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
