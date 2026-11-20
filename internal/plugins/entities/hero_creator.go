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
	// Key is what a hero-choice rule names the entry by (a package entry's
	// slug, a campaign entry's id), so a client can match a rule's entry
	// without relying on a name that may be renamed.
	Key         string         `json:"key,omitempty"`
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Summary     string         `json:"summary"`
	Source      string         `json:"source"`
	Description string         `json:"description,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	// Status is "draft" or "sent" for a player's own entry a Director has
	// not accepted yet; Mine and EntryID are set for the player who wrote it.
	Status  string `json:"status,omitempty"`
	Mine    bool   `json:"mine,omitempty"`
	EntryID int64  `json:"entryId,omitempty"`
	// State is HeroChoiceStateAskFirst on an entry the player may pick only
	// with a Director's yes; empty for an entry they may simply pick.
	State string `json:"state,omitempty"`
	// Held marks an entry the rules took off the player's list, carried only
	// so a hero that already holds it stays editable. Never served, and never
	// a valid new pick.
	Held bool `json:"-"`
}

// HeroStep is one question: a character field and the entries it offers.
type HeroStep struct {
	FieldKey string       `json:"fieldKey"`
	Label    string       `json:"label"`
	Choices  []HeroChoice `json:"choices"`
	// CanMake: a Director let this player write their own entry here.
	CanMake bool `json:"canMake,omitempty"`
	// Rules says what the hero-choice rules did to the step for this player;
	// nil when none applies.
	Rules *HeroStepRules `json:"rules,omitempty"`
	// Held is what the rules took off the list, kept server-side so a hero
	// that holds one stays editable. Never served.
	Held []HeroChoice `json:"-"`
}

// HeroPlan is every step for one campaign, in the order the system declares
// its character fields.
type HeroPlan struct {
	SystemID   string     `json:"systemId,omitempty"`
	SystemName string     `json:"systemName"`
	Steps      []HeroStep `json:"steps"`
	// Practice marks the plan a practice hero is made from: every entry the
	// rules keep from this player is back in the lists, labelled, and no rule
	// locks or forces a step.
	Practice bool `json:"practice,omitempty"`
}

// HeroPlanner supplies a campaign's creator steps. director says whether the
// viewer may see entries the Directors kept from players; userID lets a
// player see the entries kept to a character they play.
type HeroPlanner interface {
	HeroPlan(ctx context.Context, campaignID, userID string, director bool) (*HeroPlan, error)
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
// a player may make their own when the owner switched on character claiming,
// the hero's page type is one players may claim, and they do not already own
// a character: one hero at a time, so a slip cannot fill the party list.
type heroAccess struct {
	Allowed bool
	ForSelf bool
	// HasHero marks a refusal whose one reason is that the player already
	// owns a hero, so the page can say so instead of "not allowed".
	HasHero bool
	// OwnHero is that hero, set by the handler for the link.
	OwnHero *Entity
}

func heroAccessFor(role campaigns.Role, claimingOn bool, target *EntityType, ownsCharacter bool) heroAccess {
	if target == nil {
		return heroAccess{}
	}
	if role >= campaigns.RoleScribe {
		return heroAccess{Allowed: true}
	}
	if role >= campaigns.RolePlayer && claimingOn && isClaimableType(target) {
		if ownsCharacter {
			return heroAccess{HasHero: true}
		}
		return heroAccess{Allowed: true, ForSelf: true}
	}
	return heroAccess{}
}

// refusalMessage is why the creator is closed to this viewer: one hero at a
// time when that is the reason, otherwise the campaign does not offer it.
func (a heroAccess) refusalMessage() string {
	if a.HasHero {
		return "you already have a hero in this campaign; players have one hero at a time"
	}
	return "you can't create a hero in this campaign"
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
			if !isObj || strings.TrimSpace(name) == "" || !hasCost || cost < 0 {
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

// heroPickResult is one step's pick after it has been checked against the
// plan: the entry it names (as the plan spells it) and the options bought.
type heroPickResult struct {
	choice *HeroChoice
	bought []string
}

// resolveHeroPicks checks every pick against the plan and returns, by step
// key, the entry each names and the options bought from it, normalised to the
// plan's spelling. Unknown steps and entries, options the entry does not
// offer, duplicates and overspending are refused. A step sent with no name is
// skipped. Both the real hero and a practice hero are checked here, so the
// two cannot drift on what a valid pick is.
//
// An ask-first entry's options are checked only when checkAsked is set: the
// real hero never stores an ask-first pick, so what was bought beside it is
// dropped with it, while a practice hero stores the pick as it is.
func resolveHeroPicks(plan *HeroPlan, picks map[string]HeroPick, checkAsked bool) (map[string]heroPickResult, error) {
	steps := map[string]*HeroStep{}
	if plan != nil {
		for i := range plan.Steps {
			steps[plan.Steps[i].FieldKey] = &plan.Steps[i]
		}
	}
	out := make(map[string]heroPickResult, len(picks))
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
		res := heroPickResult{choice: choice}
		if len(pick.Options) > 0 && (checkAsked || choice.State != HeroChoiceStateAskFirst) {
			costs, budget, ok := heroBuyList(choice.Properties)
			if !ok {
				return nil, apperror.NewBadRequest(choice.Name + " has nothing to choose from")
			}
			seen := map[string]bool{}
			spent := 0.0
			res.bought = make([]string, 0, len(pick.Options))
			for _, o := range pick.Options {
				k := strings.ToLower(strings.TrimSpace(o))
				cost, known := costs[k]
				if !known || seen[k] {
					return nil, apperror.NewBadRequest(fmt.Sprintf("%q can't be chosen for %s", o, choice.Name))
				}
				seen[k] = true
				spent += cost
				res.bought = append(res.bought, strings.TrimSpace(o))
			}
			if spent > budget {
				return nil, apperror.NewBadRequest(fmt.Sprintf("those choices cost %g points; %s has %g", spent, choice.Name, budget))
			}
		}
		out[key] = res
	}
	return out, nil
}

// buildHeroFields checks every pick against the plan and returns the hero's
// fields: each step's field holds the chosen entry's name, as the sheet and
// the Foundry module expect, and bought options sit in the field beside it.
//
// A pick of an ask-first entry is not stored: it comes back in asks, to be
// recorded as a request once the hero exists. A step the rules require can't
// be skipped; when it is left with one entry that entry is filled in, so the
// hero never rests on the client having sent it.
func buildHeroFields(plan *HeroPlan, picks map[string]HeroPick) (fields map[string]any, asks []HeroAsk, err error) {
	resolved, err := resolveHeroPicks(plan, picks, false)
	if err != nil {
		return nil, nil, err
	}
	fields = map[string]any{}
	asked := map[string]bool{}
	for key, r := range resolved {
		if r.choice.State == HeroChoiceStateAskFirst {
			asks = append(asks, HeroAsk{FieldKey: key, EntryName: r.choice.Name})
			asked[key] = true
			continue
		}
		fields[key] = r.choice.Name
		if len(r.bought) == 0 {
			continue
		}
		raw, err := json.Marshal(r.bought)
		if err != nil {
			return nil, nil, apperror.NewInternal(err)
		}
		fields[key+heroOptionsSuffix] = string(raw)
	}
	if plan != nil {
		for i := range plan.Steps {
			step := &plan.Steps[i]
			if step.Rules == nil || !step.Rules.Required || asked[step.FieldKey] {
				continue
			}
			if _, has := fields[step.FieldKey]; has {
				continue
			}
			if step.Rules.Prefilled != "" {
				fields[step.FieldKey] = step.Rules.Prefilled
				continue
			}
			return nil, nil, apperror.NewBadRequest(fmt.Sprintf("choose the %s; it can't be skipped", strings.ToLower(step.Label)))
		}
	}
	sort.Slice(asks, func(i, j int) bool { return asks[i].FieldKey < asks[j].FieldKey })
	return fields, asks, nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
