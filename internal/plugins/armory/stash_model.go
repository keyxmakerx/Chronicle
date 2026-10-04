// stash_model.go holds the domain types for stashes and item/money moves.
//
// Money is carried as whole cents (Cents) everywhere inside the plugin so a
// sum or a comparison never meets float rounding; it is converted to the game
// system's own numeric field value only at the character boundary.
package armory

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

// Move kinds, endpoint kinds and statuses. The strings are the stored ENUM
// values of item_moves, so they must not be renamed without a migration.
const (
	MoveKindItem  = "item"
	MoveKindMoney = "money"

	EndpointCharacter = "character"
	EndpointStash     = "stash"

	MoveApplied  = "applied"
	MovePending  = "pending"
	MoveDeclined = "declined"
	MoveFailed   = "failed"
)

// Limits that keep stash names and places readable and the history bounded.
const (
	maxStashName     = 120
	maxStashLocation = 200
	maxMoveQuantity  = 1000000
	historyPageSize  = 50
	panelHistorySize = 3
)

// Cents is a money amount in hundredths of the game system's currency unit.
type Cents int64

// ParseCents reads a plain decimal amount ("12", "12.5", "12.50") into cents.
// It rejects anything that is not a positive number with at most two decimals,
// so an exponent, a sign, a thousands separator or a third decimal can never
// slip a different amount past the check.
func ParseCents(s string) (Cents, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, apperror.NewBadRequest("Enter an amount.")
	}
	whole, frac, hasDot := strings.Cut(s, ".")
	if whole == "" && !hasDot {
		return 0, apperror.NewBadRequest("Enter an amount.")
	}
	if whole == "" {
		whole = "0"
	}
	if len(frac) > 2 {
		return 0, apperror.NewBadRequest("Amounts can have at most two decimal places.")
	}
	for _, part := range []string{whole, frac} {
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0, apperror.NewBadRequest("Enter the amount as a plain number, like 12 or 12.50.")
			}
		}
	}
	if len(whole) > 12 {
		return 0, apperror.NewBadRequest("That amount is too large.")
	}
	for len(frac) < 2 {
		frac += "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, apperror.NewBadRequest("Enter the amount as a plain number, like 12 or 12.50.")
	}
	c := Cents(w*100) + Cents(mustAtoi(frac))
	if c <= 0 {
		return 0, apperror.NewBadRequest("The amount must be more than zero.")
	}
	return c, nil
}

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// Decimal renders cents as the exact "12.50" string MariaDB DECIMAL expects.
func (c Cents) Decimal() string {
	neg := ""
	v := int64(c)
	if v < 0 {
		neg, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%02d", neg, v/100, v%100)
}

// String renders cents for people: whole amounts without decimals.
func (c Cents) String() string {
	if c%100 == 0 {
		return strconv.FormatInt(int64(c)/100, 10)
	}
	return c.Decimal()
}

// Endpoint names one side of a move: a character entity or a stash.
type Endpoint struct {
	Kind string
	ID   string
}

// Valid reports whether the endpoint names a known kind with an id.
func (e Endpoint) Valid() bool {
	return (e.Kind == EndpointCharacter || e.Kind == EndpointStash) && e.ID != ""
}

// StashID returns the numeric stash id for a stash endpoint.
func (e Endpoint) StashID() (int, bool) {
	if e.Kind != EndpointStash {
		return 0, false
	}
	n, err := strconv.Atoi(e.ID)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// StashEndpoint builds the endpoint for a stash id.
func StashEndpoint(id int) Endpoint { return Endpoint{Kind: EndpointStash, ID: strconv.Itoa(id)} }

// Stash is a named holding place shared by a campaign.
type Stash struct {
	ID         int
	CampaignID string
	Name       string
	Location   string // Empty when none is set.
	Money      Cents
	CreatedBy  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// StashItemRow is one item line in a stash.
type StashItemRow struct {
	ItemEntityID string
	Quantity     int
}

// Move is one row of the move history; a pending row is a request.
type Move struct {
	ID           int64
	CampaignID   string
	Kind         string
	ItemEntityID string // Empty for money.
	Quantity     int    // 0 for money.
	Amount       Cents  // 0 for items.
	From         Endpoint
	To           Endpoint
	Status       string
	Reason       string
	RequestedBy  string
	DecidedBy    string
	CreatedAt    time.Time
	DecidedAt    *time.Time

	// byGM is set when the requester was Owner or Scribe. It is not stored: a
	// request that waits for approval is always a player's, so an unset value
	// is the safe reading (never touch a DM-only line).
	byGM bool
}

// IsMoneyEdit reports whether the row records a character's money being
// changed on its sheet rather than a move: a money row whose two ends are the
// same character.
func (m Move) IsMoneyEdit() bool {
	return m.Kind == MoveKindMoney && m.From.Kind == EndpointCharacter && m.From == m.To
}

// IsGive reports whether the row records the GM handing an item to a
// character: an item row whose two ends are the same character. It was
// applied the moment it was written and never runs again, so no apply path may
// debit or credit it a second time.
func (m Move) IsGive() bool {
	return m.Kind == MoveKindItem && m.From.Kind == EndpointCharacter && m.From == m.To
}

// MoveFilter narrows a history query. Zero values mean "no restriction".
type MoveFilter struct {
	// Endpoint, when set, keeps moves that touch it on either side.
	Endpoint *Endpoint
	// RequestedBy, when set, keeps only that user's moves.
	RequestedBy string
	// Status, when set, keeps only that status.
	Status string
	Limit  int
}

// Downtime is the per-campaign switch that decides whether moves apply at once.
type Downtime struct {
	IsOpen    bool
	ChangedBy string
	ChangedAt time.Time
}

// MoveInput is what a player (or the GM) asks for.
type MoveInput struct {
	Kind         string
	ItemEntityID string
	Quantity     int
	Amount       string // Plain decimal text; parsed by ParseCents.
	From         Endpoint
	To           Endpoint
}

// CreateStashInput creates a stash.
type CreateStashInput struct {
	Name     string
	Location string
}

// UpdateStashInput is a PARTIAL update: an absent field keeps the stored
// value, an explicit null clears a nullable one, a present value replaces it.
// A rename must never wipe the location.
type UpdateStashInput struct {
	Name     patch.Field[string]
	Location patch.Field[string]
}

// EntityRef is the little the plugin needs to know about a character or item
// entity. The adapter in internal/app fills it so the service never touches
// the entities plugin.
type EntityRef struct {
	ID          string
	Name        string
	TypeID      int
	OwnerUserID string // Empty when unclaimed.
	// IsCharacter is true for the whole character family, player characters
	// included. IsItem is true for item-category entities.
	IsCharacter bool
	IsItem      bool
	// MoneyKey is the character's money field key, empty when its entity type
	// has no usable numeric money field.
	MoneyKey string
	// MoneyLabel is that field's display label, empty when it has none.
	MoneyLabel string
	// Purse maps each 5e coin (cp, sp, ep, gp, pp) to the sheet field that
	// holds it. It is set only when the type has numeric gp, sp and cp, so
	// change can always be given; MoneyKey stays "gp" so stash moves keep treating gp as the
	// character's money.
	Purse map[string]string
}

// MoveLine is a history row with names resolved for display.
type MoveLine struct {
	Move
	ItemName      string
	FromName      string
	ToName        string
	RequesterName string
	// Summary, when set, is the whole sentence for the row. Purchase requests
	// share the history list and carry their own wording here.
	Summary string
}

// HeldItem is one line of what a character carries.
type HeldItem struct {
	ItemID   string
	Name     string
	Quantity int
}

// CharacterPanelView feeds the "Items and money" panel on a character page.
type CharacterPanelView struct {
	CampaignID   string
	Character    EntityRef
	DowntimeOpen bool
	// CanGive is true for Owner visibility, who may hand the character items
	// and maps.
	CanGive     bool
	Items       []HeldItem
	HasMoney    bool
	Money       Cents
	History     []MoveLine
	HistoryMore bool
}

// NamedRef is an id with a display name.
type NamedRef struct {
	ID   string
	Name string
}

// StashItemView is one item line in a stash with its name resolved.
type StashItemView struct {
	ItemID   string
	Name     string
	Quantity int
}

// StashView is a stash as one viewer sees it.
type StashView struct {
	Stash
	Viewers []NamedRef
	Items   []StashItemView
}

// Hidden reports whether only the GM can see the stash.
func (v StashView) Hidden() bool { return len(v.Viewers) == 0 }

// StashesPageView feeds the stashes page.
type StashesPageView struct {
	CampaignID   string
	DowntimeOpen bool
	CanManage    bool // Owner or Scribe.
	CanApprove   bool // Owner visibility.
	Pending      []MoveLine
	// PendingPurchases are shop baskets waiting for the Owner, listed beside
	// the move requests.
	PendingPurchases []PurchaseRequestLine
	Stashes          []StashView
	Characters       []NamedRef // Character-family entities, for the viewer picker.
	Items            []NamedRef // Catalogue items the GM can drop into a stash.
}

// WaitingCount is every request waiting on the Owner: moves and purchases.
func (v *StashesPageView) WaitingCount() int { return len(v.Pending) + len(v.PendingPurchases) }

// MoveDestination is one choice in the "To" select.
type MoveDestination struct {
	Endpoint Endpoint
	Label    string
	Group    string // "Stashes" or "Characters".
}

// MoveDialogView feeds the move dialog.
type MoveDialogView struct {
	CampaignID   string
	Kind         string
	ItemID       string
	ItemName     string
	From         Endpoint
	FromName     string
	Max          int   // Most of an item that can be moved.
	MaxMoney     Cents // Most money that can be moved.
	Destinations []MoveDestination
	Immediate    bool // True when the move happens at once.
}

// MoveOutcome reports what Move did.
type MoveOutcome struct {
	Move    Move
	Applied bool // False when it became a request.
}
