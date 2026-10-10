package systems

import (
	"context"
	"strings"
)

// maxCreatorSteps bounds how many questions the hero creator asks, so a
// system with dozens of listed text fields still gives a walkable creator.
const maxCreatorSteps = 12

// CreatorOptionsSuffix names the field beside a step's field that keeps what
// a hero bought from the chosen entry's list (ancestry_choices_json).
const CreatorOptionsSuffix = "_choices_json"

// CreatorStep is one question the hero creator asks: a text field on the
// system's character preset that has entries to pick from, whether the
// package lists them or the campaign's Directors added their own.
type CreatorStep struct {
	FieldKey string   `json:"fieldKey"`
	Label    string   `json:"label"`
	Choices  []Choice `json:"choices"`
}

// CreatorPlan is everything the creator needs to draw its steps. Steps is
// empty when neither the system nor the campaign lists anything yet, and the
// creator then asks only for a name.
type CreatorPlan struct {
	SystemID   string        `json:"systemId"`
	SystemName string        `json:"systemName"`
	Steps      []CreatorStep `json:"steps"`
}

// CreatorPlan lists the campaign system's character fields that have
// entries, in the order the preset declares them. The viewer recorded with
// WithChoiceViewer decides whether Directors-only entries are included.
func (s *characterChoiceService) CreatorPlan(ctx context.Context, campaignID string) (*CreatorPlan, error) {
	plan := &CreatorPlan{Steps: []CreatorStep{}}
	var preset *EntityPresetDef
	for _, c := range s.candidates(ctx, campaignID) {
		if p := characterPresetOf(c.manifest); p != nil {
			preset = p
			plan.SystemID, plan.SystemName = c.manifest.ID, c.manifest.Name
			break
		}
	}
	if preset == nil {
		return plan, nil
	}
	for _, f := range preset.Fields {
		// A key ending in the creator's options suffix would collide with the
		// field that keeps another step's bought options.
		if f.Type != "string" || !ValidChoiceFieldKey(f.Key) || strings.HasSuffix(f.Key, CreatorOptionsSuffix) {
			continue
		}
		l, err := s.CharacterChoiceList(ctx, campaignID, f.Key)
		if err != nil {
			return nil, err
		}
		if len(l.Choices) == 0 {
			continue
		}
		label := strings.TrimSpace(f.Label)
		if label == "" {
			label = f.Key
		}
		plan.Steps = append(plan.Steps, CreatorStep{FieldKey: f.Key, Label: label, Choices: l.Choices})
		if len(plan.Steps) == maxCreatorSteps {
			break
		}
	}
	return plan, nil
}

// characterPresetOf prefers the preset a system marks as its character and
// falls back to the older "-character" slug convention.
func characterPresetOf(m *SystemManifest) *EntityPresetDef {
	if p := m.CharacterPresetByCategory(); p != nil {
		return p
	}
	return m.CharacterPreset()
}
