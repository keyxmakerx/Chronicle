package entities

// tryout_plan.go shapes the plan a look-around visitor builds from. The
// visitor writes nothing to the server, so the plan is the one thing the
// server says to them, and it says only what a public reader of the campaign
// can already read.

// tryoutPlan rebuilds a visitor's plan from an allow-list of fields, so a field
// added to HeroPlan later is private to visitors until someone decides
// otherwise. The input is already the list a new player with no rules of their
// own is offered (VisitorPlanner); this is the second lock on it.
//
// What stays: each step's key and label, and each entry's name, summary and
// text. What goes: every rule detail (not-here names and reasons, the held
// list, waiting and suggested asks), the "make your own" offer, an ask-first
// seal (the entry is a plain choice here, since nobody can be asked), and any
// entry that is a player's own or a draft. A step left with nothing to pick
// is dropped, since without its rule note it would be a dead end.
func tryoutPlan(plan *HeroPlan) *HeroPlan {
	out := &HeroPlan{Steps: []HeroStep{}, Practice: true, Tryout: true}
	if plan == nil {
		return out
	}
	out.SystemID, out.SystemName = plan.SystemID, plan.SystemName
	for _, st := range plan.Steps {
		step := HeroStep{FieldKey: st.FieldKey, Label: st.Label, Choices: []HeroChoice{}, Needed: st.Needed, Parent: st.Parent, CountFrom: st.CountFrom}
		for _, c := range st.Choices {
			if c.Mine || c.Status != "" || c.Held || (c.Source != "package" && c.Source != "campaign") {
				continue
			}
			step.Choices = append(step.Choices, HeroChoice{
				Key: c.Key, Slug: c.Slug, Name: c.Name, Summary: c.Summary, Source: c.Source,
				Description: c.Description, Properties: c.Properties, Parent: c.Parent,
			})
		}
		if len(step.Choices) > 0 {
			out.Steps = append(out.Steps, step)
		}
	}
	return out
}
