package fieldrules

import (
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Limits keep a campaign's tools small enough to evaluate on every page view.
const (
	MaxRules     = 100
	MaxRuleRows  = 60
	MaxWords     = 500
	MaxNameLen   = 100
	MaxGivesLen  = 100
	maxIsLen     = 100
	maxFollowLen = 100
)

// Rule works out a field's value from another field: the first row that
// matches the followed field's value gives the result.
type Rule struct {
	ID         int       `json:"id"`
	CampaignID string    `json:"-"`
	Name       string    `json:"name"`
	Follows    string    `json:"follows"` // the followed field's key
	Rows       []RuleRow `json:"rows"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// RuleRow matches either one value (Is) or a range of numbers (From and/or
// To, both inclusive; one left out is open-ended).
type RuleRow struct {
	Is    string   `json:"is,omitempty"`
	From  *float64 `json:"from,omitempty"`
	To    *float64 `json:"to,omitempty"`
	Gives string   `json:"gives"`
}

// LocalWord is what one place or one era calls a word. Exactly one of PlaceID
// and EraID is set.
type LocalWord struct {
	ID        int    `json:"id"`
	Word      string `json:"word"`
	Local     string `json:"local"`
	PlaceID   string `json:"placeId,omitempty"`
	PlaceName string `json:"placeName,omitempty"`
	EraID     int    `json:"eraId,omitempty"`
	EraName   string `json:"eraName,omitempty"`
	DMOnly    bool   `json:"dmOnly"`
}

// Viewer is who is asking. Role is the campaign visibility role, so a
// granted co-Director counts as the Director.
type Viewer struct {
	UserID string
	Role   int
}

// IsDM is true for the Director and a granted co-Director: the only people
// who change rules and words or see a word kept DM only.
func (v Viewer) IsDM() bool { return permissions.CanSeeDmOnly(v.Role) }

// RuleInput creates a rule (ID 0) or replaces one.
type RuleInput struct {
	ID      int
	Name    string
	Follows string
	Rows    []RuleRow
}

// WordInput creates a local word (ID 0) or replaces one. Exactly one of
// PlaceID and EraID is set.
type WordInput struct {
	ID      int
	Word    string
	Local   string
	PlaceID string
	EraID   int
	DMOnly  bool
}
