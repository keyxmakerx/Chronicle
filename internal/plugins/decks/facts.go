package decks

import (
	"context"
	"errors"
	"time"
)

// Facts reads what a card's tests are judged on. It is owned here and
// implemented in internal/app over the entities, trackers, tags and
// transformations services, so decks never imports another plugin.
//
// Every method answers as a Director would see it: a Director's own test may
// name a tag or a layer no player can read. A fact that cannot be read is
// reported as an error and judged as missing, and a test on a missing fact
// does not hold, so a failing source can never make a card come up.
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
}

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
	MaxTests  int          `json:"maxTests"`
}

// OptionWord is a key and the words it is said with.
type OptionWord struct {
	Key    string `json:"key"`
	Phrase string `json:"phrase"`
}
