package entities

import (
	"context"
	"log/slog"
	"time"
)

// hero_rules.go is how the hero creator and the sheet's choice boxes meet the
// campaign's hero-choice rules (limits, required picks, ask-first). The rules
// live in the systems package; this plugin sees them only through the plan it
// is handed and the HeroRules interface, wired at the composition root.

// HeroChoiceStateAskFirst marks a HeroChoice the player may pick only with a
// Director's yes.
const HeroChoiceStateAskFirst = "ask_first"

// HeroNotHere names an entry a rule keeps off a player's list.
type HeroNotHere struct {
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
}

// HeroRequestView is a pick waiting on a Director, or a Director's note
// asking for another.
type HeroRequestView struct {
	ID        int64     `json:"id"`
	EntryName string    `json:"entryName"`
	EntityID  string    `json:"entityId,omitempty"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// HeroStepRules is what the rules did to one step for this player.
type HeroStepRules struct {
	// Limited: a rule applies to this player here, so a typed value must be
	// on the list too.
	Limited bool `json:"limited"`
	// Required: the list is a short list the step must be chosen from.
	Required bool `json:"required,omitempty"`
	// Prefilled is the one entry a required list is left with: filled in and
	// locked.
	Prefilled    string           `json:"prefilled,omitempty"`
	NotHereCount int              `json:"notHereCount,omitempty"`
	NotHere      []HeroNotHere    `json:"notHere,omitempty"`
	Waiting      *HeroRequestView `json:"waiting,omitempty"`
	Suggested    *HeroRequestView `json:"suggested,omitempty"`
}

// HeroBreak is one rule a hero already breaks: it holds an entry the rules
// now keep off its player's list. The hero keeps it and stays editable.
type HeroBreak struct {
	FieldKey string `json:"fieldKey"`
	Value    string `json:"value"`
	// Kind is "not_offered" or "off_list" (not on a required short list).
	Kind   string `json:"kind"`
	Reason string `json:"reason,omitempty"`
}

// HeroWaitingPick is one blank on a hero that waits for a Director.
type HeroWaitingPick struct {
	ID        int64  `json:"id"`
	FieldKey  string `json:"fieldKey"`
	EntryName string `json:"entryName"`
}

// HeroAsk is a pick of an ask-first entry, to be recorded as a request.
type HeroAsk struct {
	CampaignID string
	UserID     string
	EntityID   string
	EntityName string
	FieldKey   string
	EntryName  string
}

// HeroAskOutcome is what a write answers with in place of storing the pick:
// the request now waits for a Director.
type HeroAskOutcome struct {
	ID        int64  `json:"id"`
	FieldKey  string `json:"fieldKey"`
	EntryName string `json:"entryName"`
	Status    string `json:"status"`
}

// HeroRules is the rules the plugin cannot read itself.
type HeroRules interface {
	// RequestChoice records a pick of an ask-first entry as a request.
	RequestChoice(ctx context.Context, in HeroAsk) (*HeroAskOutcome, error)
	// Breaks lists the rules a hero already breaks, never changing it.
	Breaks(ctx context.Context, campaignID string, e *Entity) ([]HeroBreak, error)
	// Waiting lists the picks the hero's player has waiting on a Director.
	Waiting(ctx context.Context, campaignID string, e *Entity) ([]HeroWaitingPick, error)
}

// SetHeroRules wires the hero-choice rules. Without them nothing is limited.
func (h *Handler) SetHeroRules(r HeroRules) { h.heroRules = r }

// unlimitedPlanKey marks a plan read for staff making a hero for the table,
// who are not limited.
type unlimitedPlanKey struct{}

// WithUnlimitedPlan reads the hero plan without the player limits. A Scribe
// makes heroes for the table; the hero is flagged once a player claims it if
// it breaks a rule.
func WithUnlimitedPlan(ctx context.Context) context.Context {
	return context.WithValue(ctx, unlimitedPlanKey{}, true)
}

// PlanUnlimited reports whether WithUnlimitedPlan was set.
func PlanUnlimited(ctx context.Context) bool {
	v, _ := ctx.Value(unlimitedPlanKey{}).(bool)
	return v
}

// addHeroRuleNotes adds what the hero page and sheet say about the rules to
// the fields response: "choice_breaks" (rules the hero already breaks; it
// keeps what it has) and "choice_waiting" (blanks waiting on a Director). Only
// the hero's own player and the Directors get them, and a failure here never
// costs the page its fields.
func (h *Handler) addHeroRuleNotes(ctx context.Context, response map[string]any, campaignID string, e *Entity, allowed bool) {
	if h.heroRules == nil || !allowed || e.OwnerUserID == nil {
		return
	}
	if breaks, err := h.heroRules.Breaks(ctx, campaignID, e); err != nil {
		slog.Warn("reading a hero's rule breaks failed", slog.String("entity_id", e.ID), slog.Any("error", err))
	} else if len(breaks) > 0 {
		response["choice_breaks"] = breaks
	}
	if waiting, err := h.heroRules.Waiting(ctx, campaignID, e); err != nil {
		slog.Warn("reading a hero's waiting picks failed", slog.String("entity_id", e.ID), slog.Any("error", err))
	} else if len(waiting) > 0 {
		response["choice_waiting"] = waiting
	}
}
