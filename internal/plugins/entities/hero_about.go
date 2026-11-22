package entities

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// HeroAboutField is one box of the hero creator's "About your hero" step:
// who the hero is in the player's own words. Every box is optional, and each
// is a field the player may edit on the hero later (identity_edit.go).
type HeroAboutField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Long is a paragraph rather than a line.
	Long bool `json:"long,omitempty"`
}

// heroAboutKeys are the fields the step asks for, in the order it asks.
// The step offers only those the hero's page type has, so a system that
// lacks one simply leaves its box out.
var heroAboutKeys = []string{"pronouns", "gender", "appearance", "backstory"}

// heroAbout is the step's boxes for a page type: the about fields it has
// that are neither GM-only nor missing. Nil when it has none, so the creator
// shows no step.
func heroAbout(t *EntityType) []HeroAboutField {
	if t == nil {
		return nil
	}
	have := map[string]FieldDefinition{}
	for _, f := range t.Fields {
		have[f.Key] = f
	}
	var out []HeroAboutField
	for _, k := range heroAboutKeys {
		f, ok := have[k]
		if !ok || f.GMOnly {
			continue
		}
		label := f.Label
		if label == "" {
			label = k
		}
		out = append(out, HeroAboutField{Key: k, Label: label, Long: ownerStoryFieldKeys[k]})
	}
	return out
}

// heroAboutValues checks what the creator sent for the step: only the boxes
// it offered, each within the length the hero's own edits allow. Empty boxes
// are left out, so nothing is written for them.
func heroAboutValues(about []HeroAboutField, sent map[string]string) (map[string]any, error) {
	if len(sent) == 0 {
		return nil, nil
	}
	offered := map[string]HeroAboutField{}
	for _, f := range about {
		offered[f.Key] = f
	}
	out := map[string]any{}
	for k, v := range sent {
		f, ok := offered[k]
		if !ok {
			return nil, apperror.NewBadRequest(fmt.Sprintf("%q is not asked for here", k))
		}
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		max := maxOwnerIdentityValue
		if f.Long {
			max = maxOwnerStoryValue
		}
		if utf8.RuneCountInString(v) > max {
			return nil, apperror.NewBadRequest(fmt.Sprintf("%s is too long; keep it under %d characters", f.Label, max))
		}
		out[k] = v
	}
	return out, nil
}
