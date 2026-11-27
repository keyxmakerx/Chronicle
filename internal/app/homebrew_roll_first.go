package app

// homebrew_roll_first.go puts "The player rolls first" in the Homebrew
// Workshop: a step a player rolls for on their own screen. The roll log makes
// the roll on the server, as a Table roll by that player, so it shows in the
// log under the log's own audience rules; the homebrew service judges the
// total. A saved step's dice are checked with the roll log's own grammar.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolls"
)

func init() {
	addWorkshopSource("the player rolls first", func(reg *homebrew.Registry, d workshopDeps) error {
		// Without the roll log there is nothing to roll with, so the step is
		// not offered rather than offered and failing at the table.
		if d.Rolls == nil {
			return nil
		}
		return reg.RegisterAction(rollFirstAction(d.Rolls, d.PowerRolls))
	})
}

// RollFirstAction is the step's registered name.
const RollFirstAction = "roll-first"

// rollFirstMaxTarget bounds the target: well past any total the roll log's
// 50 dice and modifier can make worth asking for.
const rollFirstMaxTarget = 999

// rollFirstArgs is what a recipe stores: the dice in the roll log's grammar,
// optionally the game system's characteristic added from the player's hero,
// and the total needed to carry on.
type rollFirstArgs struct {
	Dice           string `json:"dice"`
	Characteristic string `json:"characteristic,omitempty"`
	Target         int    `json:"target"`
}

// rollTable is the part of the roll log the step needs.
type rollTable interface {
	Table(ctx context.Context, campaignID string, v rolls.Viewer, in rolls.TableInput) (*rolls.View, error)
}

// rollFirstStats reads the game system's characteristics and one player's
// numbers for them: heroID's when the player owns that page, else their first
// hero's (rollsPowerAdapter).
type rollFirstStats interface {
	PowerRollOn(ctx context.Context, campaignID, userID, heroID string, canSeeGM bool) (rolls.PowerRollLine, error)
}

type rollFirst struct {
	rolls rollTable
	// power is nil while no game system can be read, leaving plain dice.
	power rollFirstStats
}

func rollFirstAction(svc rollTable, power rollFirstStats) homebrew.ActionDef {
	rf := &rollFirst{rolls: svc, power: power}
	return homebrew.ActionDef{Name: RollFirstAction, Label: "The player rolls first",
		Fields: []homebrew.Field{
			{Name: "dice", Label: "Dice", Type: homebrew.FieldText},
			{Name: "characteristic", Label: "Adds their", Type: homebrew.FieldText, Optional: true},
			{Name: "target", Label: "Needs at least", Type: homebrew.FieldWhole},
		},
		Check: rf.check,
		Rolls: &homebrew.PlayerRoll{Offer: rf.offer, Roll: rf.roll}}
}

func (rf *rollFirst) check(ctx context.Context, campaignID string, raw json.RawMessage) (json.RawMessage, error) {
	var a rollFirstArgs
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return nil, apperror.NewValidation("the roll takes dice, a characteristic and a target")
	}
	shown, ok := rolls.CheckFormula(a.Dice)
	if !ok {
		return nil, apperror.NewValidation("Write the dice like 2d10, d20 or 2d6+1.")
	}
	a.Dice = shown
	a.Characteristic = strings.Join(strings.Fields(a.Characteristic), " ")
	if a.Characteristic != "" {
		st, _, err := rf.stat(ctx, campaignID, "", "", a.Characteristic)
		if err != nil {
			return nil, err
		}
		a.Characteristic = st.Label
	}
	if a.Target < 1 || a.Target > rollFirstMaxTarget {
		return nil, apperror.NewValidation(fmt.Sprintf("The roll needs a target from 1 to %d.", rollFirstMaxTarget))
	}
	return json.Marshal(a)
}

// stat finds a characteristic the game system declares, by its key or label
// in any case, with userID's numbers for it when userID is set (heroID's,
// when they own that page), and the page the numbers came from.
func (rf *rollFirst) stat(ctx context.Context, campaignID, userID, heroID, name string) (rolls.PowerStat, string, error) {
	none := apperror.NewValidation("This campaign's game system names no characteristics. Leave it empty and put any bonus in the dice.")
	if rf.power == nil {
		return rolls.PowerStat{}, "", none
	}
	line, err := rf.power.PowerRollOn(ctx, campaignID, userID, heroID, false)
	if err != nil {
		return rolls.PowerStat{}, "", err
	}
	if !line.Declared {
		return rolls.PowerStat{}, "", none
	}
	for _, c := range line.Characteristics {
		if strings.EqualFold(c.Key, name) || strings.EqualFold(c.Label, name) {
			return c, line.HeroID, nil
		}
	}
	names := make([]string, 0, len(line.Characteristics))
	for _, c := range line.Characteristics {
		names = append(names, c.Label)
	}
	return rolls.PowerStat{}, "", apperror.NewValidation(fmt.Sprintf("%q is not one of the game system's characteristics (%s).", name, strings.Join(names, ", ")))
}

// work is what the player would roll: the dice plus their hero's number
// for the characteristic, and that hero's page. The hero is the page the
// rule's event happened on when the player owns it, else their first hero.
func (rf *rollFirst) work(ctx context.Context, campaignID string, act homebrew.Act) (homebrew.RollOffer, string, error) {
	var a rollFirstArgs
	if err := json.Unmarshal(act.Args, &a); err != nil {
		return homebrew.RollOffer{}, "", apperror.NewValidation("this roll's settings could not be read")
	}
	o := homebrew.RollOffer{Formula: a.Dice, Target: a.Target}
	heroID := ""
	if a.Characteristic != "" {
		st, hero, err := rf.stat(ctx, campaignID, act.By.UserID, act.Scope, a.Characteristic)
		if err != nil {
			return o, "", err
		}
		if st.Value == nil {
			return o, "", apperror.NewValidation(fmt.Sprintf("Your hero's page has no %s number, so this roll can't be made here.", st.Label))
		}
		heroID = hero
		o.Label, o.Formula = st.Label, withBonus(a.Dice, *st.Value)
	}
	shown, ok := rolls.CheckFormula(o.Formula)
	if !ok {
		return o, "", apperror.NewValidation("These dice can't be rolled. Your Director can change them.")
	}
	o.Formula = shown
	return o, heroID, nil
}

func withBonus(dice string, n int) string {
	switch {
	case n > 0:
		return dice + "+" + strconv.Itoa(n)
	case n < 0:
		return dice + "-" + strconv.Itoa(-n)
	}
	return dice
}

func (rf *rollFirst) offer(ctx context.Context, campaignID string, act homebrew.Act) (homebrew.RollOffer, error) {
	o, _, err := rf.work(ctx, campaignID, act)
	return o, err
}

// houseRuleRollLabel names a roll-first roll with no characteristic.
const houseRuleRollLabel = "House rule roll"

// roll makes the roll as a public Table roll by the player, as their hero
// when the characteristic came from one, so it lands in the roll log and
// on everyone's Table like any roll they make.
func (rf *rollFirst) roll(ctx context.Context, campaignID string, act homebrew.Act) (homebrew.Rolled, error) {
	o, heroID, err := rf.work(ctx, campaignID, act)
	if err != nil {
		return homebrew.Rolled{}, err
	}
	// The roll is public, so its label never names the recipe: a Director
	// may call a rule after a twist the players haven't met yet.
	label := o.Label
	if label == "" {
		label = houseRuleRollLabel
	}
	v, err := rf.rolls.Table(ctx, campaignID, rolls.Viewer{UserID: act.By.UserID, Name: act.By.Name, Director: act.By.Director},
		rolls.TableInput{Formula: o.Formula, Label: label, Mode: rolls.ModePublic, CharacterID: heroID})
	if err != nil {
		return homebrew.Rolled{}, err
	}
	if v == nil || v.Total == nil {
		return homebrew.Rolled{}, apperror.NewInternal(fmt.Errorf("roll first: the roll log gave no total"))
	}
	return homebrew.Rolled{RollOffer: o, Total: *v.Total, RollID: v.ID}, nil
}

// homebrewScopePlayers finds who plays the page a rule's event happened on:
// the member who claimed it, on a page of this campaign.
type homebrewScopePlayers struct {
	entities interface {
		GetByID(ctx context.Context, id string) (*entities.Entity, error)
	}
}

func (p *homebrewScopePlayers) PlayerOf(ctx context.Context, campaignID, pageID string) string {
	if p.entities == nil || pageID == "" {
		return ""
	}
	// Any failure leaves the roll waiting on whoever set the rule off.
	e, err := p.entities.GetByID(ctx, pageID)
	if err != nil || e == nil || e.CampaignID != campaignID || e.OwnerUserID == nil {
		return ""
	}
	return *e.OwnerUserID
}
