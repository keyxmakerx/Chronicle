package rumours

import (
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// BlockType is the block's type in the page block registry.
const BlockType = "rumours"

// What the Director knows about a rumour. Only the Director ever reads or sets
// it.
const (
	TruthOpen  = "open"
	TruthTrue  = "true"
	TruthFalse = "false"
)

// What the party has decided to believe. BeliefNone is a rumour nobody has
// marked yet.
const (
	BeliefNone   = ""
	BeliefTrue   = "true"
	BeliefFalse  = "false"
	BeliefUnsure = "unsure"
)

// How sure the person who said it was, as shown by the three dots.
const (
	SureMin     = 1
	SureMax     = 3
	SureDefault = 2
)

// Bounds that keep a page's rumours a small thing. They are why the block can
// read a whole page in one query instead of paging it.
const (
	// QuoteMax is the longest rumour, in characters.
	QuoteMax = 280
	// SourceMax is the longest "who said it", in characters.
	SourceMax = 80
	// MaxRumours is how many rumours one page keeps. It is also the most a
	// read sends, so a page can never hold a rumour its block cannot show;
	// adding past it asks for some to be removed first.
	MaxRumours = 200
)

// Viewer is who is asking. Role is the campaign's visibility role
// (CampaignContext.VisibilityRole), so a granted co-Director counts as the
// Director; the service never compares it against anything else.
type Viewer struct {
	UserID string
	Role   int
}

// IsDM is true for the Director and a granted co-Director.
func (v Viewer) IsDM() bool { return permissions.CanSeeDmOnly(v.Role) }

// IsScribe is true for a Scribe who is not also the Director.
func (v Viewer) IsScribe() bool { return !v.IsDM() && v.Role >= permissions.RoleScribe }

// CanAdd says who may write a rumour: the Director and Scribes. Players read.
func (v Viewer) CanAdd() bool { return v.IsDM() || v.IsScribe() }

// CanMark says who may set what the party thinks.
func (v Viewer) CanMark() bool { return v.CanAdd() }

// Rumour is one stored rumour with the names of the people who touched it.
// Truth is empty when the read was made for someone who may not see it.
type Rumour struct {
	ID         string
	CampaignID string
	EntityID   string
	Quote      string
	Source     string
	Sure       int
	Truth      string
	Belief     string
	BeliefBy   string
	BeliefName string
	BeliefByDM bool
	ByDM       bool
	CreatedBy  string
	AuthorName string
	CreatedAt  time.Time
}

// RumourView is a rumour as one viewer may see it. Truth is a pointer so that
// for everyone but the Director the key is absent from the JSON altogether,
// not present and empty.
type RumourView struct {
	ID    string `json:"id"`
	Quote string `json:"quote"`
	Who   string `json:"who"`
	Sure  int    `json:"sure"`
	// When is the day it was heard, already worded for the page.
	When string `json:"when"`
	// AddedBy is "you", a person's name, or empty for a rumour the Director
	// wrote (the Director's own rumours carry no byline).
	AddedBy string `json:"addedBy,omitempty"`
	// Belief is what the party thinks; empty until someone marks it.
	Belief   string `json:"belief"`
	BeliefBy string `json:"beliefBy,omitempty"`
	// Truth is only ever set in a view built for the Director.
	Truth *string `json:"truth,omitempty"`
	// CanEdit is whether this viewer may edit or remove this rumour.
	CanEdit bool `json:"canEdit"`
}

// View is everything the block shows one viewer.
type View struct {
	// Hidden means the block draws nothing for this viewer: a player on a page
	// with no rumours has nothing to read and no way to add one.
	Hidden bool `json:"-"`
	// IsDM is true for the Director, the only viewer sent the truth.
	IsDM bool `json:"isDM"`
	// CanAdd and CanMark are what this viewer may do; the server re-checks
	// both on every write.
	CanAdd  bool `json:"canAdd"`
	CanMark bool `json:"canMark"`
	// Total is how many rumours the page holds. When it exceeds the length of
	// Rumours the read was cut off, and the block says so.
	Total   int          `json:"total"`
	Rumours []RumourView `json:"rumours"`
}

// Cut reports whether the read sent fewer rumours than the page holds.
func (v *View) Cut() bool { return v != nil && v.Total > len(v.Rumours) }

// AddInput is a new rumour.
type AddInput struct {
	CampaignID string
	EntityID   string
	Quote      string
	Who        string
	Sure       int
	Viewer     Viewer
}

// UpdateRumourInput edits what was said, who said it or how sure they were.
// Each field is a patch.Field so a body naming one of them leaves the others
// as they are. All three are required columns, so an explicit null preserves.
// Which rumour, on which page, and who is asking travel beside it.
type UpdateRumourInput struct {
	Quote patch.Field[string]
	Who   patch.Field[string]
	Sure  patch.Field[int]
}

// cleanText trims a free-text field, folds inner whitespace runs and refuses
// control characters. It reports the cleaned text and whether it fits max
// characters; an over-long or control-laden value is rejected, never cut
// quietly, so the person sees what was too long.
func cleanText(s string, max int) (string, bool) {
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
