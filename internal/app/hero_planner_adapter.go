package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities/cardborders"
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
			Choices: heroChoices(cctx, s.Choices), Held: heroChoices(cctx, s.Excluded), Rules: heroStepRules(s.Rules),
			Needed: s.Needed, Parent: s.Parent, CountFrom: heroCounts(s.CountFrom),
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
		out.Steps = append(out.Steps, entities.HeroStep{
			FieldKey: s.FieldKey, Label: s.Label, Choices: heroChoices(ctx, s.Choices),
			Needed: s.Needed, Parent: s.Parent, CountFrom: heroCounts(s.CountFrom),
		})
	}
	return out, nil
}

// heroChoices copies a list into the plan. What the game system itself gives
// a card, and whether the owner has changed it, are copied only for a
// Director (ctx): the Customizer needs them, and a player's plan has no use
// for them.
func heroChoices(ctx context.Context, in []systems.Choice) []entities.HeroChoice {
	director := systems.ChoiceViewerIsDirector(ctx)
	out := make([]entities.HeroChoice, 0, len(in))
	for _, c := range in {
		props, look := heroLook(c.Properties)
		var sysLook *entities.HeroLook
		changed := false
		if director && c.LookChanged {
			changed = true
			if c.SystemLook != nil {
				sysLook = &entities.HeroLook{Border: c.SystemLook.Border, Colour: c.SystemLook.Colour}
			}
		}
		out = append(out, entities.HeroChoice{
			SystemLook: sysLook, LookChanged: changed,
			Key: c.Key, Slug: c.Slug, Name: c.Name, Summary: c.Summary, Source: c.Source,
			Description: c.Description, Properties: props, Look: look,
			Status: c.Status, Mine: c.Mine, EntryID: c.EntryID, State: c.State, Parent: c.Parent,
		})
	}
	return out
}

// heroLook lifts the card border an entry names out of its properties. The
// properties it returns never hold the two border keys, so the creator does
// not list them as facts; they are a copy, because the entry's own map is
// shared and cached. An entry that names no (valid) border still loses the
// two keys and gets no look.
func heroLook(in map[string]any) (map[string]any, *entities.HeroLook) {
	_, hasBorder := in[cardborders.PropBorder]
	_, hasColour := in[cardborders.PropColour]
	if !hasBorder && !hasColour {
		return in, nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if k != cardborders.PropBorder && k != cardborders.PropColour {
			out[k] = v
		}
	}
	l, ok := cardborders.LookFrom(in)
	if !ok {
		return out, nil
	}
	return out, &entities.HeroLook{Border: l.Border, Colour: l.Colour}
}

func heroCounts(in []systems.CreatorCount) []entities.HeroCount {
	if len(in) == 0 {
		return nil
	}
	out := make([]entities.HeroCount, 0, len(in))
	for _, c := range in {
		out = append(out, entities.HeroCount{Step: c.Step, Property: c.Property, Note: c.Note})
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

// heroTermsAdapter gives the hero builder its rules words from the systems
// package's Rules book glossary, so the entities plugin never imports it.
type heroTermsAdapter struct {
	choices systems.CharacterChoiceService
}

// HeroTerms copies the words across; the systems side has already made them
// plain text and left out anything Director-only.
func (a heroTermsAdapter) HeroTerms(ctx context.Context, campaignID string) (*entities.HeroTerms, error) {
	t, err := a.choices.RulesTerms(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := &entities.HeroTerms{Terms: make(map[string]entities.HeroTerm, len(t.Terms)), Present: t.Present}
	for k, v := range t.Terms {
		out.Terms[k] = entities.HeroTerm{Name: v.Name, Category: v.Category, Text: v.Text}
	}
	return out, nil
}

// heroPickChecker gives AI Import's review the hero creator's check of a
// hero's pick-list values, over the same plan the creator draws, so the
// review and the creator cannot disagree on what is on a list.
type heroPickChecker struct {
	planner entities.HeroPlanner
}

// HeroPickWarnings is words for each written field that is off the list. A
// Director is checked against the whole list, the hero-choice rules not
// applied: the owner is reviewing what the AI proposed, not playing the
// creator. Anyone else gets the plan their own account would see. A plan
// that can't be read gives no warning.
func (c heroPickChecker) HeroPickWarnings(ctx context.Context, campaignID, userID string, director bool, fields map[string]any, written []string) []string {
	if director {
		ctx = entities.WithUnlimitedPlan(ctx)
	}
	plan, err := c.planner.HeroPlan(ctx, campaignID, userID, director)
	if err != nil {
		return nil
	}
	return entities.HeroFieldWarnings(plan, fields, written)
}
