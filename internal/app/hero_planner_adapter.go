package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// heroPlannerAdapter gives the entities plugin's hero creator its steps from
// the systems pick lists, so the plugin never imports the systems package
// (which already imports entities).
type heroPlannerAdapter struct {
	choices systems.CharacterChoiceService
}

// HeroPlan reads the plan as the account userID gets it. The campaign's
// hero-choice rules apply to that account unless the viewer is a Director, or
// staff making a hero for the table (entities.WithUnlimitedPlan).
func (a heroPlannerAdapter) HeroPlan(ctx context.Context, campaignID, userID string, director bool) (*entities.HeroPlan, error) {
	cctx := systems.WithChoiceFor(systems.WithChoiceViewerUser(systems.WithChoiceViewer(ctx, director), userID), userID)
	if entities.PlanUnlimited(ctx) {
		cctx = systems.WithChoiceUnlimited(cctx)
	}
	p, err := a.choices.CreatorPlan(cctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := &entities.HeroPlan{SystemID: p.SystemID, SystemName: p.SystemName, Steps: make([]entities.HeroStep, 0, len(p.Steps))}
	for _, s := range p.Steps {
		step := entities.HeroStep{
			FieldKey: s.FieldKey, Label: s.Label, CanMake: s.CanMake,
			Choices: heroChoices(s.Choices), Held: heroChoices(s.Excluded), Rules: heroStepRules(s.Rules),
		}
		out.Steps = append(out.Steps, step)
	}
	return out, nil
}

// VisitorPlan reads the plan as a look-around visitor gets it: the list a
// brand-new player with no rules or grants of their own is offered, built from
// what the public can read (systems.WithChoiceVisitor). Rule details, held
// entries and asks are never copied across, so the plan has no way to carry
// them.
func (a heroPlannerAdapter) VisitorPlan(ctx context.Context, campaignID string) (*entities.HeroPlan, error) {
	p, err := a.choices.CreatorPlan(systems.WithChoiceVisitor(ctx), campaignID)
	if err != nil {
		return nil, err
	}
	out := &entities.HeroPlan{SystemID: p.SystemID, SystemName: p.SystemName, Steps: make([]entities.HeroStep, 0, len(p.Steps))}
	for _, s := range p.Steps {
		out.Steps = append(out.Steps, entities.HeroStep{FieldKey: s.FieldKey, Label: s.Label, Choices: heroChoices(s.Choices)})
	}
	return out, nil
}

func heroChoices(in []systems.Choice) []entities.HeroChoice {
	out := make([]entities.HeroChoice, 0, len(in))
	for _, c := range in {
		out = append(out, entities.HeroChoice{
			Key: c.Key, Slug: c.Slug, Name: c.Name, Summary: c.Summary, Source: c.Source,
			Description: c.Description, Properties: c.Properties,
			Status: c.Status, Mine: c.Mine, EntryID: c.EntryID, State: c.State,
		})
	}
	return out
}

func heroStepRules(r *systems.FieldRules) *entities.HeroStepRules {
	if r == nil {
		return nil
	}
	out := &entities.HeroStepRules{
		Limited: r.Limited, Required: r.Required, Prefilled: r.Prefilled, NotHereCount: r.NotHereCount,
		Waiting: heroRequestView(r.Waiting), Suggested: heroRequestView(r.Suggested),
	}
	for _, n := range r.NotHere {
		out.NotHere = append(out.NotHere, entities.HeroNotHere{Name: n.Name, Reason: n.Reason})
	}
	return out
}

func heroRequestView(v *systems.RequestView) *entities.HeroRequestView {
	if v == nil {
		return nil
	}
	return &entities.HeroRequestView{ID: v.ID, EntryName: v.EntryName, EntityID: v.EntityID, Message: v.Message, CreatedAt: v.CreatedAt}
}
