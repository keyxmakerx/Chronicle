package decks

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Facts reads what a card's tests are judged on. It is owned here and
// implemented in internal/app over the entities, trackers, tags,
// transformations and calendar services, so decks never imports another plugin.
//
// Every method but Today answers as a Director would see it: a Director's own
// test may name a tag or a layer no player can read. A fact that cannot be
// read is reported as an error and judged as missing, and a test on a missing
// fact does not hold, so a failing source can never make a card come up.
type Facts interface {
	// Numbers reads a page's numbers: its stored sheet values with its layers
	// applied.
	Numbers(ctx context.Context, campaignID, pageID string) (NumberSet, error)
	// Trackers lists the trackers on a page, hidden ones included.
	Trackers(ctx context.Context, campaignID, pageID string) ([]TrackerFact, error)
	// Tags lists the page's tags, Director-only tags included.
	Tags(ctx context.Context, campaignID, pageID string) ([]TagFact, error)
	// Layers lists the transformations on a page.
	Layers(ctx context.Context, campaignID, pageID string) ([]LayerFact, error)
	// Fields lists the number fields a page has: its category's and its own.
	Fields(ctx context.Context, campaignID, pageID string) ([]NumberField, error)
	// Options says what the editor may offer.
	Options(ctx context.Context, campaignID string) (WhenOptions, error)
	// Today reads the main calendar's current date as v sees it: a moon hidden
	// from v, or an event v may not see, is not in it. It is the one fact read
	// for a viewer rather than as a Director, so a player's own draw judges the
	// calendar exactly as that player is shown it. A campaign with no main
	// calendar v may see, or no current date, is ErrUnavailable.
	Today(ctx context.Context, campaignID string, v Viewer) (TodayFact, error)
}

// TodayFact is the main calendar's current date as one viewer sees it.
type TodayFact struct {
	// Moons are the moons the viewer may see, by lower-cased, trimmed name.
	Moons map[string]MoonFact
	// Events are the lower-cased names of today's events and the slugs and
	// names of their kinds. Nil when they could not be read, which is a
	// missing fact, not an empty day.
	Events map[string]bool
	// DayPart is dawn, day, dusk or night; "" when the calendar can't say.
	DayPart string
}

// MoonFact is one moon on the current date. Day is the day of its cycle, the
// new moon's day being 1, or 0 for a moon set by hand; Words are the phase
// words it matches today ("Waning Crescent", "waning", "crescent").
type MoonFact struct {
	Name  string
	Day   int
	Words []string
}

// calendarKey is how a moon or event name is matched: trimmed, any case.
func calendarKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// ErrUnavailable is returned by Facts when the plugin a fact comes from is
// not running. It is judged as a missing fact without a log line, so a
// degraded plugin does not fill the log after every page save.
var ErrUnavailable = errors.New("decks: that fact is not available")

// NumberSet is a page's numbers as the server can know them.
//
// The server judges stored values and the page's number layers. A number the
// sheet only works out in the browser is not here, so a test on it reads as
// missing.
type NumberSet struct {
	// Sheet is whether the page has a stored sheet at all. A page with none
	// never satisfies a number test.
	Sheet bool
	// Values is each number field's value by key. A field that is neither
	// stored nor set by a layer is absent.
	Values map[string]float64
	// Sources says where a value came from, by key, only where the adapter
	// knows: "layer" for a value a layer set or changed. A stored value has no
	// entry, because the store does not say whether a hand or Foundry wrote it.
	Sources map[string]string
	// ChangedAt is when the page last changed.
	ChangedAt time.Time
}

// TrackerFact is one tracker on a page.
type TrackerFact struct {
	KindID string
	Name   string
	Value  int
}

// TagFact is one tag on a page.
type TagFact struct {
	ID   int
	Name string
}

// LayerFact is one transformation on a page. Source is the preset it came
// from ("" for one made by hand).
type LayerFact struct {
	Name   string
	Source string
}

// NumberField is a number field a test may name. MaxKey is the field that
// holds its upper bound, when the game system says so.
type NumberField struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	MaxKey string `json:"maxKey,omitempty"`
}

// OptionHero is a hero page a test may name.
type OptionHero struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TrackerKindOption is a tracker kind a test may name.
type TrackerKindOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EventKindOption is a kind of calendar event a test may name.
type EventKindOption struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// TagOption is a tag a test may name.
type TagOption struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// WhenOptions is what the editor may offer for a campaign: the party, the
// number fields of hero pages, and the tracker kinds and tags that exist.
type WhenOptions struct {
	Heroes   []OptionHero        `json:"heroes"`
	Fields   []NumberField       `json:"fields"`
	Trackers []TrackerKindOption `json:"trackers"`
	Tags     []TagOption         `json:"tags"`
	// Layers are the transformation names now on any hero, for the editor to
	// offer; a test may still name one that is not on a page yet.
	Layers []string `json:"layers"`
	// Calendar is whether the campaign's main calendar could be read; without
	// it no moon, event or time test can hold.
	Calendar bool `json:"hasCalendar"`
	// Moons are the main calendar's moons by name, hidden ones included.
	Moons []string `json:"moons"`
	// EventKinds are the campaign's kinds of event, for the editor to offer; an
	// event test may also name an event.
	EventKinds []EventKindOption `json:"eventKinds"`
}

// Reading is what a notice says about the first test that names a number or a
// tracker: the sentence of what it is now, where the value came from when the
// adapter knows, and when the page last changed.
type Reading struct {
	Text   string
	Source string
	At     time.Time
}

// WhenOptionsView is what the options route returns: Options plus the words
// the editor needs to say a test in a sentence.
type WhenOptionsView struct {
	WhenOptions
	// Available is false when no facts are wired, so only "the Director says
	// so" can be used.
	Available bool         `json:"available"`
	Ops       []OptionWord `json:"ops"`
	Shares    []OptionWord `json:"shares"`
	TestKinds []OptionWord `json:"testKinds"`
	// DayParts and MoonPhases are what a time and a moon test may name.
	DayParts   []OptionWord `json:"dayParts"`
	MoonPhases []OptionWord `json:"moonPhases"`
	MaxTests   int          `json:"maxTests"`
}

// OptionWord is a key and the words it is said with.
type OptionWord struct {
	Key    string `json:"key"`
	Phrase string `json:"phrase"`
}
