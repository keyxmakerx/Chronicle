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

func (a heroPlannerAdapter) HeroPlan(ctx context.Context, campaignID string, director bool) (*entities.HeroPlan, error) {
	p, err := a.choices.CreatorPlan(systems.WithChoiceViewer(ctx, director), campaignID)
	if err != nil {
		return nil, err
	}
	out := &entities.HeroPlan{SystemName: p.SystemName, Steps: make([]entities.HeroStep, 0, len(p.Steps))}
	for _, s := range p.Steps {
		step := entities.HeroStep{FieldKey: s.FieldKey, Label: s.Label, Choices: make([]entities.HeroChoice, 0, len(s.Choices))}
		for _, c := range s.Choices {
			step.Choices = append(step.Choices, entities.HeroChoice{
				Slug: c.Slug, Name: c.Name, Summary: c.Summary, Source: c.Source,
				Description: c.Description, Properties: c.Properties,
			})
		}
		out.Steps = append(out.Steps, step)
	}
	return out, nil
}
