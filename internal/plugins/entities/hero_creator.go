package entities

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// hero_creator.go is the logic behind the hero creator: which page type a new
// hero becomes, who may make one, and turning the creator's picks into the
// hero's fields. The steps themselves come from the game system through
// HeroPlanner, wired at the composition root, so this plugin never reads
// system packages.

// HeroChoice is one entry a step offers. Description and Properties are the
// entry's own text and data for the creator's leaf; Source is "package" for
// the system's entries and "campaign" for the Directors' own.
type HeroChoice struct {
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Summary     string         `json:"summary"`
	Source      string         `json:"source"`
	Description string         `json:"description,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
}

// HeroStep is one question: a character field and the entries it offers.
type HeroStep struct {
	FieldKey string       `json:"fieldKey"`
	Label    string       `json:"label"`
	Choices  []HeroChoice `json:"choices"`
}

// HeroPlan is every step for one campaign, in the order the system declares
// its character fields.
type HeroPlan struct {
	SystemName string     `json:"systemName"`
	Steps      []HeroStep `json:"steps"`
}

// HeroPlanner supplies a campaign's creator steps. director says whether the
// viewer may see entries the Directors kept from players.
type HeroPlanner interface {
	HeroPlan(ctx context.Context, campaignID string, director bool) (*HeroPlan, error)
}

// HeroPick is what the creator sends for one step: the chosen entry's name
// and, when the entry has a list to buy from, the names bought.
type HeroPick struct {
	Name    string   `json:"name"`
	Options []string `json:"options,omitempty"`
}

// HeroRequest is the creator's finished hero.
type HeroRequest struct {
	Name  string              `json:"name"`
	Picks map[string]HeroPick `json:"picks"`
}

// heroAccess says whether the viewer may use the creator and, if so, whether
// the hero becomes theirs. Scribes and above make heroes for the table;
// a player may make their own when the owner switched on character claiming
// and the hero's page type is one players may claim.
type heroAccess struct {
	Allowed bool
	ForSelf bool
}

func heroAccessFor(role campaigns.Role, claimingOn bool, target *EntityType) heroAccess {
	if target == nil {
		return heroAccess{}
	}
	if role >= campaigns.RoleScribe {
		return heroAccess{Allowed: true}
	}
	if role >= campaigns.RolePlayer && claimingOn && isClaimableType(target) {
		return heroAccess{Allowed: true, ForSelf: true}
	}
	return heroAccess{}
}

// pickHeroType chooses the page type a new hero becomes, from the types the
// owner listed as characters (charTypeIDs, already resolved). It prefers the
// game system's character type, then the player-character type, then the
// first listed. A player's hero must be a type players may claim.
func pickHeroType(types []EntityType, charTypeIDs []int, claimableOnly bool) *EntityType {
	byID := make(map[int]*EntityType, len(types))
	for i := range types {
		byID[types[i].ID] = &types[i]
	}
	var listed []*EntityType
	for _, id := range charTypeIDs {
		if t := byID[id]; t != nil && t.Enabled && (!claimableOnly || isClaimableType(t)) {
			listed = append(listed, t)
		}
	}
	for _, t := range listed {
		if t.PresetCategory != nil && *t.PresetCategory == "character" {
			return t
		}
	}
	for _, t := range listed {
		if isPCType(*t) {
			return t
		}
	}
	if len(listed) > 0 {
		return listed[0]
	}
	return nil
}

// heroOptionsSuffix names the field that keeps what a hero bought from an
// entry's list, next to the field holding the entry: ancestry and
// ancestry_choices_json. A JSON array of names, like the system's other
// *_json text fields.
const heroOptionsSuffix = "_choices_json"

// heroBuyList finds the list an entry lets a hero buy from: the first
// property that is a list of named items with a numeric cost, with the budget
// in a numeric property ending in "_points". Without a budget nothing can be
// bought, and the creator only shows the list.
func heroBuyList(props map[string]any) (costs map[string]float64, budget float64, ok bool) {
	keys := sortedKeys(props)
	for _, k := range keys {
		if n, isNum := props[k].(float64); isNum && n > 0 && strings.HasSuffix(k, "_points") {
			budget = n
			break
		}
	}
	if budget == 0 {
		return nil, 0, false
	}
	for _, k := range keys {
		items, isList := props[k].([]any)
		if !isList || len(items) == 0 {
			continue
		}
		found := map[string]float64{}
		for _, it := range items {
			m, isObj := it.(map[string]any)
			name, _ := m["name"].(string)
			cost, hasCost := m["cost"].(float64)
			if !isObj || strings.TrimSpace(name) == "" || !hasCost {
				found = nil
				break
			}
			found[strings.ToLower(strings.TrimSpace(name))] = cost
		}
		if found != nil {
			return found, budget, true
		}
	}
	return nil, 0, false
}

// buildHeroFields checks every pick against the plan and returns the hero's
// fields: each step's field holds the chosen entry's name, as the sheet and
// the Foundry module expect, and bought options sit in the field beside it.
func buildHeroFields(plan *HeroPlan, picks map[string]HeroPick) (map[string]any, error) {
	fields := map[string]any{}
	steps := map[string]*HeroStep{}
	if plan != nil {
		for i := range plan.Steps {
			steps[plan.Steps[i].FieldKey] = &plan.Steps[i]
		}
	}
	for key, pick := range picks {
		step := steps[key]
		if step == nil {
			return nil, apperror.NewBadRequest("the creator has no step for " + key)
		}
		name := strings.TrimSpace(pick.Name)
		if name == "" {
			if len(pick.Options) > 0 {
				return nil, apperror.NewBadRequest("choose the " + strings.ToLower(step.Label) + " before its options")
			}
			continue
		}
		var choice *HeroChoice
		for i := range step.Choices {
			if strings.EqualFold(step.Choices[i].Name, name) {
				choice = &step.Choices[i]
				break
			}
		}
		if choice == nil {
			return nil, apperror.NewBadRequest(fmt.Sprintf("%q is not on the %s list", name, strings.ToLower(step.Label)))
		}
		fields[key] = choice.Name
		if len(pick.Options) == 0 {
			continue
		}
		costs, budget, ok := heroBuyList(choice.Properties)
		if !ok {
			return nil, apperror.NewBadRequest(choice.Name + " has nothing to choose from")
		}
		seen := map[string]bool{}
		spent := 0.0
		bought := make([]string, 0, len(pick.Options))
		for _, o := range pick.Options {
			k := strings.ToLower(strings.TrimSpace(o))
			cost, known := costs[k]
			if !known || seen[k] {
				return nil, apperror.NewBadRequest(fmt.Sprintf("%q can't be chosen for %s", o, choice.Name))
			}
			seen[k] = true
			spent += cost
			bought = append(bought, strings.TrimSpace(o))
		}
		if spent > budget {
			return nil, apperror.NewBadRequest(fmt.Sprintf("those choices cost %g points; %s has %g", spent, choice.Name, budget))
		}
		raw, err := json.Marshal(bought)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		fields[key+heroOptionsSuffix] = string(raw)
	}
	return fields, nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
