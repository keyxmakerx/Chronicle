package entities

import (
	"time"
	"unicode"
	"unicode/utf8"
)

// The Standing block says how a page (a guild, a faction, a person) feels about
// the party or about one character. The Director sets the word by hand and adds
// reasons that explain it; a reason may move the word. A reason can be marked
// for the Director only, and players see the word only when the page allows it.

// StandingBlockType is the block's type in the block registry.
const StandingBlockType = "standing"

// StandingPartyWire is the subject key the API and the block use for the whole
// party. Storage keeps it as the empty string (StandingPartyKey) so it can sit
// in the same unique key as a character's page id.
const (
	StandingPartyWire = "party"
	StandingPartyKey  = ""
)

// The five steps of a standing, lowest first. The level a standing stores is an
// index into this list.
const (
	StandingLevelMin     = 0
	StandingLevelMax     = 4
	StandingLevelDefault = 2
)

var standingWords = [...]string{"Hostile", "Wary", "Neutral", "Warm", "Trusted"}

// StandingWord is the word for a level; an out-of-range level reads as Neutral
// rather than failing a page render.
func StandingWord(level int) string {
	if level < StandingLevelMin || level > StandingLevelMax {
		return standingWords[StandingLevelDefault]
	}
	return standingWords[level]
}

// Reason directions: what a reason did to the word.
const (
	StandingUp   = "up"
	StandingDown = "down"
	StandingNone = "none"
)

// Bounds that keep a page's standing a small thing. They are why the block can
// read everything for a page in a few queries instead of paging.
const (
	// StandingReasonMax is the longest reason text, in characters.
	StandingReasonMax = 90
	// StandingWhenMax is the longest "when" label, in characters.
	StandingWhenMax = 30
	// MaxStandingReasons is how many reasons one standing keeps; adding past it
	// asks the Director to remove some first.
	MaxStandingReasons = 300
	// standingReasonsShown is how many of the newest reasons one subject sends
	// to the page; Total still counts them all.
	standingReasonsShown = 200
	// maxStandingSubjects bounds the subjects read for one page.
	maxStandingSubjects = 120
	// standingCharacterPick bounds the characters offered to the Director.
	standingCharacterPick = 60
)

// StandingRow is one stored standing: a page's word for one subject.
type StandingRow struct {
	ID         string
	CampaignID string
	EntityID   string
	// SubjectKey is StandingPartyKey for the party, else a character's page id.
	SubjectKey string
	Level      int
	UpdatedBy  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// StandingReason is one stored explanation.
type StandingReason struct {
	ID         string
	StandingID string
	Direction  string
	Text       string
	When       string
	DMOnly     bool
	CreatedBy  string
	CreatedAt  time.Time
}

// StandingSubjectRef is a character the Director can set a standing toward.
type StandingSubjectRef struct {
	Key  string
	Name string
}

// StandingReasonView is a reason as one viewer may see it. DMOnly is only ever
// true in a view built for the Director: a player's list never contains a
// DM-only reason at all, so there is nothing to mark.
type StandingReasonView struct {
	ID        string `json:"id"`
	Direction string `json:"direction"`
	Text      string `json:"text"`
	When      string `json:"when"`
	DMOnly    bool   `json:"dmOnly,omitempty"`
}

// StandingSubjectView is one subject's standing as one viewer may see it.
type StandingSubjectView struct {
	// Key is "party" or a character's page id.
	Key  string `json:"key"`
	Name string `json:"name"`
	// Set is false for a subject the Director has not given a word yet; only
	// the Director is ever sent one.
	Set   bool   `json:"set"`
	Level int    `json:"level"`
	Word  string `json:"word"`
	// Total is how many reasons this viewer can see, which may exceed the
	// length of Reasons (the newest standingReasonsShown are sent).
	Total   int                  `json:"total"`
	Reasons []StandingReasonView `json:"reasons"`
}

// StandingView is everything the block shows one viewer.
type StandingView struct {
	// Hidden means the block draws nothing for this viewer: a player on a page
	// whose standing is Director-only, or that has none to show.
	Hidden bool `json:"-"`
	// CanEdit is true for the Director.
	CanEdit bool `json:"canEdit"`
	// PlayersSee is the page's switch. It is only sent to the Director: a
	// player who gets the block is by definition allowed to see it.
	PlayersSee *bool                 `json:"playersSee,omitempty"`
	Subjects   []StandingSubjectView `json:"subjects"`
}

// AddStandingReasonInput is a new reason from the Director.
type AddStandingReasonInput struct {
	CampaignID string
	EntityID   string
	// Subject is "party" or a character's page id.
	Subject   string
	Direction string
	Text      string
	When      string
	DMOnly    bool
	// Level is the word the standing becomes; nil keeps the current word (or
	// Neutral for a subject with none yet).
	Level  *int
	Role   int
	UserID string
}

// cleanStandingText trims a free-text field, folds inner whitespace runs and
// refuses control characters. It reports the cleaned text and whether it fits
// max characters; an over-long or control-laden value is rejected, never cut
// quietly, so the Director sees what was too long.
func cleanStandingText(s string, max int) (string, bool) {
	out := make([]rune, 0, len(s))
	space := false
	for _, r := range s {
		switch {
		case unicode.IsControl(r) && !unicode.IsSpace(r):
			return "", false
		case unicode.IsSpace(r):
			space = len(out) > 0
		default:
			if space {
				out = append(out, ' ')
				space = false
			}
			out = append(out, r)
		}
	}
	t := string(out)
	return t, utf8.RuneCountInString(t) <= max
}
