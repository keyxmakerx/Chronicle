package fieldrules

import (
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Limits keep a campaign's tools small enough to evaluate on every page view.
const (
	MaxRules      = 100
	MaxRuleRows   = 60
	MaxWords      = 500
	MaxNameLen    = 100
	MaxGivesLen   = 100
	maxIsLen      = 100
	maxFollowLen  = 100
	MaxRuleAdjust = 10
	maxTagLen     = 100
)

// What a rule can follow besides a field on the same page. A field key never
// holds a colon or a space, so these never collide with one.
const (
	// FollowMoonPrefix starts "moon:<moon name>": the moon's phase on the
	// campaign calendar's current date.
	FollowMoonPrefix = "moon:"
	// FollowTrackerPrefix starts "tracker:<tracker name>": the value of a
	// tracker on the same page.
	FollowTrackerPrefix = "tracker:"
	// FollowTimeOfDay follows the calendar's time of day: dawn, day, dusk or
	// night.
	FollowTimeOfDay = "time of day"
)

// Parts of the day a row's time condition or a "time of day" rule matches,
// as the calendar names them.
var dayParts = map[string]bool{"dawn": true, "day": true, "dusk": true, "night": true}

// Rule works out a field's value from another field: the first row that
// matches the followed field's value gives the result.
type Rule struct {
	ID         int       `json:"id"`
	CampaignID string    `json:"-"`
	Name       string    `json:"name"`
	Follows    string    `json:"follows"` // the followed field's key
	Rows       []RuleRow `json:"rows"`
	// Adjust changes a number the rows gave, each step whose test holds in
	// order: "1 lower while Hunger is 1 or more, never below 1".
	Adjust    []RuleAdjust `json:"adjust,omitempty"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

// RuleRow matches one value (Is) or a range of numbers (From and/or To, both
// inclusive; one left out is open-ended), and only while its conditions hold:
// an event on today's date (EventTag) and the time of day (Time). A row with
// conditions and no value matches whatever the rule follows, so a row like
// "on a Bloodnight, 1" put first overrides the rest.
//
// For a moon, Is names a phase ("Waning Crescent", or a broad word such as
// "full" or "crescent") and a range counts days of the cycle, the new moon's
// day being day 1. A moon's range may wrap, from 23 to 1.
type RuleRow struct {
	Is       string   `json:"is,omitempty"`
	From     *float64 `json:"from,omitempty"`
	To       *float64 `json:"to,omitempty"`
	EventTag string   `json:"eventTag,omitempty"`
	Time     string   `json:"time,omitempty"`
	Gives    string   `json:"gives"`
}

// RuleAdjust adds Add to a numeric result while a tracker on the same page
// passes its test (Op against Value), then keeps the result within Min and
// Max. A tracker that can't be read fails the test.
type RuleAdjust struct {
	Tracker string   `json:"tracker"`
	Op      string   `json:"op"` // gte, gt, lte, lt or eq
	Value   float64  `json:"value"`
	Add     float64  `json:"add"`
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
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
	Adjust  []RuleAdjust
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
