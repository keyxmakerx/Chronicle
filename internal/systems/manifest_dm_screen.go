package systems

import (
	"fmt"
	"regexp"
)

// DMScreenDef is a system's contribution to the DM Screen. Chronicle can't
// know what "health" means in each game, so the package names the sheet
// fields to show and the reference category its conditions live in.
type DMScreenDef struct {
	// Party lists the meters shown under each hero, in order.
	Party []DMScreenMeter `json:"party,omitempty"`

	// HeroSubtitle names the sheet field shown under a hero's name when it
	// is folded open (a class).
	HeroSubtitle string `json:"hero_subtitle,omitempty"`

	// HeroConditions names the sheet field listing the conditions a hero
	// has right now: a JSON list of names or {"name": …} objects, or a
	// comma-separated string.
	HeroConditions string `json:"hero_conditions,omitempty"`

	// Conditions points at the reference entries the rules tab lists.
	Conditions *DMScreenConditions `json:"conditions,omitempty"`
}

// DMScreenMeter is one number (or current/max pair) read from a hero's
// character-sheet fields.
type DMScreenMeter struct {
	// Label is the fixed text shown before the value ("Stamina").
	Label string `json:"label,omitempty"`

	// LabelField names a sheet field whose value is the label instead, for
	// numbers whose name depends on the hero (a class's heroic resource).
	LabelField string `json:"label_field,omitempty"`

	// Current is the sheet field holding the value. Required.
	Current string `json:"current"`

	// Max is the sheet field holding the maximum; when set, the meter draws
	// as a bar.
	Max string `json:"max,omitempty"`

	// WarnBelow marks the bar as low when current/max drops under this
	// fraction (0.5 for Draw Steel's winded). Zero disables the warning.
	WarnBelow float64 `json:"warn_below,omitempty"`
}

// DMScreenConditions selects conditions from one of the system's reference
// categories, optionally narrowed to entries whose property equals a value.
type DMScreenConditions struct {
	Category string `json:"category"`
	Property string `json:"property,omitempty"`
	Value    string `json:"value,omitempty"`
}

const maxDMScreenMeters = 6

// fieldKeyPattern matches the sheet field keys presets declare.
var fieldKeyPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

// validateDMScreen rejects a block the screen could not draw, so a typo in a
// package shows up at install rather than as a silently empty panel.
func validateDMScreen(d *DMScreenDef) error {
	if d == nil {
		return nil
	}
	if len(d.Party) > maxDMScreenMeters {
		return fmt.Errorf("too many party meters (%d, max %d)", len(d.Party), maxDMScreenMeters)
	}
	for i, m := range d.Party {
		if !fieldKeyPattern.MatchString(m.Current) {
			return fmt.Errorf("party meter %d: current must be a sheet field key", i)
		}
		if m.Max != "" && !fieldKeyPattern.MatchString(m.Max) {
			return fmt.Errorf("party meter %d: max must be a sheet field key", i)
		}
		if m.LabelField != "" && !fieldKeyPattern.MatchString(m.LabelField) {
			return fmt.Errorf("party meter %d: label_field must be a sheet field key", i)
		}
		if m.Label == "" && m.LabelField == "" {
			return fmt.Errorf("party meter %d: label or label_field is required", i)
		}
		if m.WarnBelow < 0 || m.WarnBelow > 1 {
			return fmt.Errorf("party meter %d: warn_below must be between 0 and 1", i)
		}
	}
	if d.HeroSubtitle != "" && !fieldKeyPattern.MatchString(d.HeroSubtitle) {
		return fmt.Errorf("hero_subtitle must be a sheet field key")
	}
	if d.HeroConditions != "" && !fieldKeyPattern.MatchString(d.HeroConditions) {
		return fmt.Errorf("hero_conditions must be a sheet field key")
	}
	if c := d.Conditions; c != nil {
		if !slugPattern.MatchString(c.Category) {
			return fmt.Errorf("conditions: category must be a category slug")
		}
		if (c.Property == "") != (c.Value == "") {
			return fmt.Errorf("conditions: property and value go together")
		}
	}
	return nil
}
