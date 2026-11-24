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
	// CanMake: the viewer may write their own entry for this step.
	CanMake bool `json:"canMake,omitempty"`
	// Rules says what the hero-choice rules did to this step for the viewer.
	Rules *FieldRules `json:"rules,omitempty"`
	// Excluded is what the rules took off the list, kept server-side so a
	// hero that already holds one stays editable. Never served.
	Excluded []Choice `json:"-"`
	// Needed: the system's rules expect this pick, so the creator does not
	// call it optional. The server still accepts a hero without it.
	Needed bool `json:"needed,omitempty"`
	// Parent is the field key of the step this step's entries live inside
	// (each entry carries the parent entry's name in Choice.Parent).
	Parent string `json:"parent,omitempty"`
	// CountFrom says how many of this step the chosen parent entries allow;
	// see CreatorCount.
	CountFrom []CreatorCount `json:"countFrom,omitempty"`
	// Multi: the step takes as many picks as CountFrom gives (CreatorDef.Multi).
	Multi bool `json:"multi,omitempty"`
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
// entries: the ones the preset's creator_order names first, in that order,
// then the rest in the order the preset declares them. The viewer recorded with
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
	for _, f := range creatorOrdered(preset) {
		// A key ending in the creator's options suffix would collide with the
		// field that keeps another step's bought options.
		if f.Type != "string" || !ValidChoiceFieldKey(f.Key) || strings.HasSuffix(f.Key, CreatorOptionsSuffix) {
			continue
		}
		l, err := s.CharacterChoiceList(ctx, campaignID, f.Key)
		if err != nil {
			return nil, err
		}
		// A limited step stays even with nothing offered, or a typed value
		// would slip past the list the rules leave.
		limited := l.Rules != nil && l.Rules.Limited
		if len(l.Choices) == 0 && !l.CanMake && !limited {
			continue
		}
		label := strings.TrimSpace(f.Label)
		if label == "" {
			label = f.Key
		}
		step := CreatorStep{FieldKey: f.Key, Label: label, Choices: l.Choices, CanMake: l.CanMake, Rules: l.Rules, Excluded: l.Excluded}
		if c := f.Creator; c != nil {
			step.Needed = c.Needed
			if c.From != nil {
				step.Parent = c.From.Step
			}
			step.CountFrom = c.CountFrom
			step.Multi = c.Multi
		}
		plan.Steps = append(plan.Steps, step)
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

// creatorOrdered returns the preset's fields with the ones creator_order
// names first, in that order. A name that matches no field is ignored and a
// repeated name counts once, so a stale declaration never drops a step.
func creatorOrdered(p *EntityPresetDef) []FieldDef {
	if len(p.CreatorOrder) == 0 {
		return p.Fields
	}
	out := make([]FieldDef, 0, len(p.Fields))
	used := map[string]bool{}
	for _, k := range p.CreatorOrder {
		for _, f := range p.Fields {
			if f.Key == k && !used[k] {
				used[k] = true
				out = append(out, f)
				break
			}
		}
	}
	for _, f := range p.Fields {
		if !used[f.Key] {
			out = append(out, f)
		}
	}
	return out
}
