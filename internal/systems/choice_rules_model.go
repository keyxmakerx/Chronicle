package systems

import (
	"context"
	"time"
)

// Hero-choice rules let a Director limit, or require, what players pick from a
// character pick list (ancestry, class, kit...). A rule is one mark on one
// entry, or on a whole group of entries, for everyone or for one player. The
// package's own entries and the campaign's homebrew are marked the same way.

const (
	// RuleOpen is a per-player exception: the entry is offered to that
	// player whatever a rule for everyone says. For everyone it is the
	// default and is never stored.
	RuleOpen = "open"
	// RuleAskFirst offers the entry, but picking it goes to a Director to
	// allow or answer with another suggestion.
	RuleAskFirst = "ask_first"
	// RuleNotOffered takes the entry off the list.
	RuleNotOffered = "not_offered"
	// RuleRequired makes the entry part of the short list a step must be
	// chosen from.
	RuleRequired = "required"

	// RuleGroupKey as an entry key marks every entry of a source at once,
	// including entries added later.
	RuleGroupKey = "*"

	// RequestWaiting is a pick that waits for a Director.
	RequestWaiting = "waiting"
	// RequestAllowed is a request a Director said yes to.
	RequestAllowed = "allowed"
	// RequestSuggested is a request a Director closed with a note asking the
	// player to choose something else.
	RequestSuggested = "suggested"
	// RequestWithdrawn is a request that no longer has anything to decide,
	// because its entry was deleted.
	RequestWithdrawn = "withdrawn"
)

// ChoiceRule is one stored mark.
type ChoiceRule struct {
	ID         int64  `json:"id"`
	CampaignID string `json:"-"`
	SystemID   string `json:"-"`
	FieldKey   string `json:"fieldKey"`
	// Source is ChoiceSourcePackage or ChoiceSourceCampaign.
	Source string `json:"source"`
	// Key is a package entry's slug, a campaign entry's id, or RuleGroupKey.
	Key string `json:"key"`
	// Name is the entry's name when the mark was last saved, so a package
	// entry that later leaves the package can still be named.
	Name string `json:"name,omitempty"`
	// Target is "" for everyone, else the player's account id.
	Target string `json:"target"`
	State  string `json:"state"`
	// Reason is shown to the player the mark applies to.
	Reason    string    `json:"reason,omitempty"`
	CreatedBy string    `json:"-"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// IsGroup reports whether the rule marks a whole source rather than an entry.
func (r ChoiceRule) IsGroup() bool { return r.Key == RuleGroupKey }

// ChoiceRequest is a hero pick that needs a Director's yes. It is kept here,
// never in the hero's fields, which the sheet and Foundry read as real.
type ChoiceRequest struct {
	ID         int64  `json:"id"`
	CampaignID string `json:"-"`
	SystemID   string `json:"-"`
	FieldKey   string `json:"fieldKey"`
	Source     string `json:"source"`
	Key        string `json:"key"`
	// Name is the entry the player asked for.
	Name   string `json:"entryName"`
	UserID string `json:"userId"`
	// EntityID and EntityName are the hero the pick is for; empty when the
	// hero page is gone.
	EntityID   string     `json:"entityId,omitempty"`
	EntityName string     `json:"entityName,omitempty"`
	Status     string     `json:"status"`
	Message    string     `json:"message,omitempty"`
	DecidedBy  string     `json:"-"`
	CreatedAt  time.Time  `json:"createdAt"`
	DecidedAt  *time.Time `json:"decidedAt,omitempty"`
}

// RequestView is a request as the player who made it reads it.
type RequestView struct {
	ID        int64  `json:"id"`
	EntryName string `json:"entryName"`
	EntityID  string `json:"entityId,omitempty"`
	// Message is the Director's note on a suggested request.
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// NotHere names an entry a rule keeps off the viewer's list, with the
// Director's reason. Only entries the viewer can read are ever named.
type NotHere struct {
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
}

// FieldRules is what the viewer needs to know about the rules on one pick
// list: whether it is limited, whether it is a required short list (and the
// one entry it is filled in with), what is kept off it, and their own
// waiting request. It is nil for a viewer no rule applies to.
type FieldRules struct {
	// Limited says some rule applies to this viewer on this list, so a typed
	// value must be on the list too.
	Limited bool `json:"limited"`
	// Required says the list is a short list the step must be chosen from.
	Required bool `json:"required,omitempty"`
	// Prefilled is the one entry a required list is left with: filled in
	// and locked.
	Prefilled    string    `json:"prefilled,omitempty"`
	NotHereCount int       `json:"notHereCount,omitempty"`
	NotHere      []NotHere `json:"notHere,omitempty"`
	// Waiting is the viewer's pick that waits for a Director.
	Waiting *RequestView `json:"waiting,omitempty"`
	// Suggested is a recent Director's note asking for another pick.
	Suggested *RequestView `json:"suggested,omitempty"`
}

// ChoiceLimiter narrows a viewer's pick list by the campaign's rules. The
// character choice service calls it after every source is merged, so the
// plan, the picker, the AI's brief and every write check read one list.
type ChoiceLimiter interface {
	Limit(ctx context.Context, campaignID string, list *ChoiceList) error
}

// unlimitedKey marks a context that reads a pick list as it is, rules aside:
// Directors and Scribes (staff are trusted) and the rules screens themselves.
type unlimitedKey struct{}

// WithChoiceUnlimited reads the pick list without the campaign's rules.
func WithChoiceUnlimited(ctx context.Context) context.Context {
	return context.WithValue(ctx, unlimitedKey{}, true)
}

// choiceUnlimited reports whether the context asked for the list as it is.
func choiceUnlimited(ctx context.Context) bool {
	v, _ := ctx.Value(unlimitedKey{}).(bool)
	return v
}

// choiceForKey carries the account a hero is for when that is not the person
// reading, such as a Director previewing the lists as one player.
type choiceForKey struct{}

// WithChoiceFor records the account the pick list is for. Rules about a
// player apply to this account; without it they apply to the viewer.
func WithChoiceFor(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, choiceForKey{}, userID)
}

// ChoiceFor is the account the pick list is for: what WithChoiceFor recorded,
// else the viewer.
func ChoiceFor(ctx context.Context) string {
	if v, _ := ctx.Value(choiceForKey{}).(string); v != "" {
		return v
	}
	return ChoiceViewerUser(ctx)
}
