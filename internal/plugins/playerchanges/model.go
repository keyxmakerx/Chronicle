package playerchanges

import (
	"strings"
	"time"
)

// Kinds of change.
const (
	KindField   = "field"   // a sheet field
	KindText    = "text"    // the page's title or text
	KindRemoved = "removed" // the page went to the Trash
)

// Sources of a change, as the page names them. They mirror the change
// source an entity write carried.
const (
	SourceWeb = "web"
	SourceAI  = "ai"
)

// Limits.
const (
	// LiveDays is how long a change stays one row of its own, answering at
	// once, before it is packed with the rest of its month.
	LiveDays = 30
	// maxValueChars is the most of a value kept as text.
	maxValueChars = 2000
	// maxJSONBytes is the most of a value kept for Undo; a bigger value is
	// recorded without it, so it cannot be undone from here.
	maxJSONBytes = 64 << 10
	// MaxResults is the most changes one search returns.
	MaxResults = 300
)

// KeepChoices are the Long-term trash lengths the site admin may pick, in
// months; 0 is off.
var KeepChoices = []int{0, 3, 6, 12, 24}

// DefaultKeepMonths is the Long-term trash length until the admin sets one.
const DefaultKeepMonths = 6

// Change is one thing a player changed.
type Change struct {
	ID         int64      `json:"id"`
	CampaignID string     `json:"-"`
	EntityID   string     `json:"e"`
	PageName   string     `json:"p"`
	UserID     string     `json:"u"`
	UserName   string     `json:"un"`
	Kind       string     `json:"k"`
	FieldKey   string     `json:"fk,omitempty"`
	FieldLabel string     `json:"fl,omitempty"`
	OldText    string     `json:"ot,omitempty"`
	NewText    string     `json:"nt,omitempty"`
	OldJSON    string     `json:"oj,omitempty"`
	NewJSON    string     `json:"nj,omitempty"`
	Source     string     `json:"s,omitempty"`
	App        string     `json:"a,omitempty"`
	CreatedAt  time.Time  `json:"t"`
	UndoneAt   *time.Time `json:"ud,omitempty"`
	UndoneBy   string     `json:"ub,omitempty"`
	// Packed is true for a change read from a packed month.
	Packed bool `json:"-"`
}

// ByAI reports whether the player's AI made the change.
func (c Change) ByAI() bool { return c.Source == SourceAI }

// What names what changed, as a line starts: the field, "Page text" or
// "Name".
func (c Change) What() string {
	switch c.Kind {
	case KindText:
		if c.FieldKey == "name" {
			return "Name"
		}
		return "Page text"
	case KindRemoved:
		return "Removed the page"
	}
	if c.FieldLabel != "" {
		return c.FieldLabel
	}
	return c.FieldKey
}

// haystack is everything a typed word is matched against.
func (c Change) haystack() string {
	return strings.ToLower(strings.Join([]string{c.PageName, c.UserName, c.What(), c.FieldKey, c.OldText, c.NewText, c.App}, " "))
}

// Query is what the owner asked the history for. Every part narrows.
type Query struct {
	// Pages are page ids; a change on any of them matches.
	Pages []string
	// Fields are field keys or "name"/"text"; a change to any matches.
	Fields []string
	// AI keeps only changes the player's AI made.
	AI bool
	// Removed keeps only removed pages.
	Removed bool
	// Words must all appear somewhere in the change.
	Words []string
	// Since is how far back to look.
	Since time.Time
}

// matches applies the whole query to one change.
func (q Query) matches(c Change) bool {
	if c.CreatedAt.Before(q.Since) {
		return false
	}
	if len(q.Pages) > 0 && !contains(q.Pages, c.EntityID) {
		return false
	}
	if len(q.Fields) > 0 && !contains(q.Fields, fieldToken(c)) {
		return false
	}
	if q.AI && !c.ByAI() {
		return false
	}
	if q.Removed && c.Kind != KindRemoved {
		return false
	}
	if len(q.Words) > 0 {
		h := c.haystack()
		for _, w := range q.Words {
			if !strings.Contains(h, strings.ToLower(w)) {
				return false
			}
		}
	}
	return true
}

// fieldToken is the tag value a change's field is searched by: its key for
// a sheet field, "name" or "text" for the page's own.
func fieldToken(c Change) string {
	switch c.Kind {
	case KindText:
		if c.FieldKey == "name" {
			return "name"
		}
		return "text"
	case KindRemoved:
		return ""
	}
	return c.FieldKey
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Result is a search's answer.
type Result struct {
	Changes []Change
	// Truncated is true when more matched than MaxResults.
	Truncated bool
	// Unpacked is how many packed months were read.
	Unpacked int
}

// Suggestion is a tag the search box offers.
type Suggestion struct {
	Kind  string `json:"kind"` // "page", "field", "ai", "removed"
	Value string `json:"value"`
	Label string `json:"label"`
}

// clip cuts s to n runes.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
