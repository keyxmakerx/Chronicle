package records

import (
	"context"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// CategorySwitchAPI is a game system's one-time field switch, as the owner
// answers it on the category's list or in the Customizer's Fields tab.
type CategorySwitchAPI interface {
	// IsOwner reports whether userID owns the campaign; only the owner
	// switches a category's fields.
	IsOwner(ctx context.Context, campaignID, userID string) (bool, error)
	// Switches lists the switches the campaign's game system offers.
	Switches(ctx context.Context, campaignID string) ([]CategorySwitch, error)
	// Accept puts the system's fields on the category.
	Accept(ctx context.Context, campaignID string, typeID int, userID string) error
	// Back puts the category's own fields back.
	Back(ctx context.Context, campaignID string, typeID int, userID string) error
}

// CategorySwitch is one category's switch and its answer.
type CategorySwitch struct {
	TypeID          int
	Category        string // the category's plural name, e.g. "Items"
	System          string // the game system's name
	Label           string // what the fields are called, e.g. "treasure fields"
	Adds            string // what the switch adds, in a line
	Removes         string // what it takes off and keeps aside, in a line; "" for nothing
	Switched        bool
	CanSwitchBack   bool
	OwnerAnswered   bool // the owner said yes, "Not now", or switched back
	SwitchedAtBirth bool // made with the system's fields; nothing to go back to
}

// The words `use:` takes.
const (
	switchUseSystem = "system"
	switchUseOwn    = "own"
)

// CategorySwitchKind lets an AI ask for a category's game system fields, or
// for its own fields back. The owner reviews it like every other row; the
// switch keeps every value aside, so nothing is lost either way.
type CategorySwitchKind struct {
	Svc CategorySwitchAPI
}

func (CategorySwitchKind) Name() string  { return "category-switch" }
func (CategorySwitchKind) Label() string { return "Game system fields" }
func (CategorySwitchKind) Brief() string {
	return "switch a category to its game system's fields, or back"
}
func (CategorySwitchKind) Doc() string {
	return "A game system can offer a category its own fields once, such as Draw Steel's treasure fields for Items. " +
		"Keys: `category`, the category's name; `use`, either `system` (the system's fields) or `own` (the fields it had before). " +
		"Switching takes the fields the system replaces off every page and keeps what they held in a fold only the Director sees; switching back restores them, so nothing is deleted either way. " +
		"New fields start empty on existing pages: fill them on each page afterwards, never in this row. Only the campaign's owner can switch. " +
		"The export below says which categories have a switch and where each stands.\n\n```\n---\nkind: category-switch\ncategory: Items\nuse: system\n---\n```"
}

// Title names the review row after its category.
func (CategorySwitchKind) Title(r Record) string {
	if c := strings.TrimSpace(r.Str("category")); c != "" {
		return c
	}
	return "Game system fields"
}

func (k CategorySwitchKind) isOwner(ctx context.Context, campaignID string, a Actor) (bool, error) {
	if !a.CanAuthorDmOnly() {
		return false, nil
	}
	return k.Svc.IsOwner(ctx, campaignID, a.UserID)
}

// resolve finds the record's category among the offered switches and reads
// its `use:`.
func (k CategorySwitchKind) resolve(ctx context.Context, campaignID string, r Record) (CategorySwitch, string, error) {
	name := strings.TrimSpace(r.Str("category"))
	if name == "" {
		name = strings.TrimSpace(r.Name)
	}
	if name == "" {
		return CategorySwitch{}, "", badRequestf("name the category in `category:`")
	}
	use := strings.ToLower(strings.TrimSpace(r.Str("use")))
	if use != switchUseSystem && use != switchUseOwn {
		return CategorySwitch{}, "", badRequestf("`use:` is `system` (the game system's fields) or `own` (the fields it had before)")
	}
	all, err := k.Svc.Switches(ctx, campaignID)
	if err != nil {
		return CategorySwitch{}, "", err
	}
	for _, s := range all {
		if sameName(s.Category, name) {
			return s, use, nil
		}
	}
	return CategorySwitch{}, "", badRequestf("the game system offers no fields for a category called %s", quote(name))
}

func (k CategorySwitchKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if r.Action == ActionDelete {
		return Plan{Error: "a switch can't be removed; use `use: own` to put the category's own fields back"}
	}
	if owner, err := k.isOwner(ctx, campaignID, a); err != nil {
		return Plan{Error: planError(err)}
	} else if !owner {
		return Plan{Error: "only the campaign's owner can switch a category's fields"}
	}
	s, use, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	if use == switchUseSystem {
		if s.Switched {
			return Plan{Error: s.Category + " already use " + s.System + "’s " + s.Label}
		}
		p := Plan{Summary: s.Category + " switch to " + s.System + "’s " + s.Label + ". " + s.Adds}
		if s.Removes != "" {
			p.Summary += " " + s.Removes
		}
		p.Warnings = append(p.Warnings, "the new fields start empty on every existing page")
		return p
	}
	if !s.Switched {
		return Plan{Error: s.Category + " already have their own fields"}
	}
	if !s.CanSwitchBack {
		return Plan{Error: s.Category + " have had " + s.System + "’s " + s.Label + " from the start, so there is nothing to switch back to"}
	}
	return Plan{Summary: s.Category + " go back to the fields they had before, with the values kept aside put back. " + s.System + "’s values are kept aside the same way."}
}

func (k CategorySwitchKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	s, use, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return err
	}
	if use == switchUseSystem {
		return k.Svc.Accept(ctx, campaignID, s.TypeID, a.UserID)
	}
	return k.Svc.Back(ctx, campaignID, s.TypeID, a.UserID)
}

// Export says where each offered switch stands, for the owner only: the AI
// should know a category's fields can change, and must not ask again for an
// answer the owner already gave.
func (k CategorySwitchKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if owner, err := k.isOwner(ctx, campaignID, a); err != nil || !owner {
		return "", err
	}
	all, err := k.Svc.Switches(ctx, campaignID)
	if err != nil || len(all) == 0 {
		return "", err
	}
	var b strings.Builder
	for _, s := range all {
		use := switchUseOwn
		if s.Switched {
			use = switchUseSystem
		}
		b.WriteString("---\nkind: category-switch\ncategory: " + quote(s.Category) + "\nuse: " + use + "\n---\n")
		switch {
		case s.Switched && s.SwitchedAtBirth:
			b.WriteString(s.Category + " have had " + s.System + "’s " + s.Label + " from the start.\n\n")
		case s.Switched:
			b.WriteString(s.Category + " use " + s.System + "’s " + s.Label + "; the owner can switch back.\n\n")
		case s.OwnerAnswered:
			b.WriteString("The owner said Not now, so " + s.Category + " keep their own fields. Suggest the switch only if they ask about " + s.System + "’s " + s.Label + ".\n\n")
		default:
			b.WriteString(s.System + " offers " + s.Category + " its " + s.Label + ". " + s.Adds + " The owner has not answered yet.\n\n")
		}
	}
	return b.String(), nil
}
