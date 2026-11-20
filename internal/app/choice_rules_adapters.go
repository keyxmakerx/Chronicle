package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// choiceHeroEntities is the part of the entities service the hero-choice
// rules need to read and set heroes.
type choiceHeroEntities interface {
	GetByID(ctx context.Context, id string) (*entities.Entity, error)
	ListRoster(ctx context.Context, campaignID string, typeIDs []int, role int, userID string) ([]entities.Entity, error)
	MergeFields(ctx context.Context, entityID string, patch map[string]any) error
}

// choiceMemberLister names the campaign's members.
type choiceMemberLister interface {
	ListMembers(ctx context.Context, campaignID string) ([]campaigns.CampaignMember, error)
}

// choiceHeroesAdapter gives the systems package's hero-choice rules the
// heroes and members they read, without that package importing entities or
// campaigns.
type choiceHeroesAdapter struct {
	entities choiceHeroEntities
	lists    entryCharacterLists
	members  choiceMemberLister
}

var _ systems.ChoiceHeroes = choiceHeroesAdapter{}

// Heroes lists the character pages a player plays, with their text fields.
func (a choiceHeroesAdapter) Heroes(ctx context.Context, campaignID string) ([]systems.ChoiceHero, error) {
	ids, err := a.lists.CharacterTypeIDs(ctx, campaignID)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	list, err := a.entities.ListRoster(ctx, campaignID, ids, int(permissions.RoleOwner), "")
	if err != nil {
		return nil, err
	}
	out := make([]systems.ChoiceHero, 0, len(list))
	for i := range list {
		if h, ok := choiceHeroOf(&list[i]); ok {
			out = append(out, h)
		}
	}
	return out, nil
}

// choiceHeroOf is a page as the rules read it; ok is false for a page nobody
// plays, which no player's rule can be about.
func choiceHeroOf(e *entities.Entity) (systems.ChoiceHero, bool) {
	if e.OwnerUserID == nil || *e.OwnerUserID == "" {
		return systems.ChoiceHero{}, false
	}
	h := systems.ChoiceHero{ID: e.ID, Name: e.Name, OwnerUserID: *e.OwnerUserID, Fields: map[string]string{}}
	for k, v := range e.FieldsData {
		if s, ok := v.(string); ok {
			h.Fields[k] = s
		}
	}
	return h, true
}

func (a choiceHeroesAdapter) Members(ctx context.Context, campaignID string) ([]systems.ChoiceMember, error) {
	if a.members == nil {
		return nil, nil
	}
	ms, err := a.members.ListMembers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]systems.ChoiceMember, 0, len(ms))
	for _, m := range ms {
		out = append(out, systems.ChoiceMember{UserID: m.UserID, Name: m.DisplayName})
	}
	return out, nil
}

// SetField writes one text field of a hero the named player still owns, as a
// Director's change. What the hero bought from the old entry is cleared, as
// it is for any change of a step.
func (a choiceHeroesAdapter) SetField(ctx context.Context, campaignID, entityID, ownerUserID, fieldKey, value string) (bool, error) {
	e, err := a.entities.GetByID(ctx, entityID)
	if err != nil || e == nil || e.CampaignID != campaignID || !e.IsOwnedBy(ownerUserID) {
		// A page that is gone, or no longer theirs, is not an error: the
		// request is still answered and the player can pick the entry.
		return false, nil
	}
	patch := map[string]any{fieldKey: value}
	entities.ClearStaleStepOptions(e, patch)
	if err := a.entities.MergeFields(ctx, entityID, patch); err != nil {
		return false, err
	}
	return true, nil
}

// heroRulesAdapter gives the entities plugin's handlers the hero-choice rules
// (systems.ChoiceRuleService) without that plugin importing systems.
type heroRulesAdapter struct {
	rules systems.ChoiceRuleService
}

var _ entities.HeroRules = heroRulesAdapter{}

func (a heroRulesAdapter) RequestChoice(ctx context.Context, in entities.HeroAsk) (*entities.HeroAskOutcome, error) {
	out, err := a.rules.RequestChoice(ctx, systems.ChoiceRequestInput{
		CampaignID: in.CampaignID, UserID: in.UserID, EntityID: in.EntityID,
		EntityName: in.EntityName, FieldKey: in.FieldKey, EntryName: in.EntryName,
	})
	if err != nil {
		return nil, err
	}
	return &entities.HeroAskOutcome{ID: out.ID, FieldKey: out.FieldKey, EntryName: out.EntryName, Status: out.Status}, nil
}

func (a heroRulesAdapter) Breaks(ctx context.Context, campaignID string, e *entities.Entity) ([]entities.HeroBreak, error) {
	h, ok := choiceHeroOf(e)
	if !ok {
		return nil, nil
	}
	list, err := a.rules.HeroBreaks(ctx, campaignID, h)
	if err != nil {
		return nil, err
	}
	out := make([]entities.HeroBreak, 0, len(list))
	for _, b := range list {
		out = append(out, entities.HeroBreak{FieldKey: b.FieldKey, Value: b.Value, Kind: b.Kind, Reason: b.Reason})
	}
	return out, nil
}

func (a heroRulesAdapter) Waiting(ctx context.Context, campaignID string, e *entities.Entity) ([]entities.HeroWaitingPick, error) {
	if e.OwnerUserID == nil {
		return nil, nil
	}
	list, err := a.rules.HeroWaiting(ctx, campaignID, *e.OwnerUserID, e.ID)
	if err != nil {
		return nil, err
	}
	out := make([]entities.HeroWaitingPick, 0, len(list))
	for _, w := range list {
		out = append(out, entities.HeroWaitingPick{ID: w.ID, FieldKey: w.FieldKey, EntryName: w.EntryName})
	}
	return out, nil
}
