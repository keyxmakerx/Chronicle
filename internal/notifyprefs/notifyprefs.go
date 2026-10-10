// Package notifyprefs holds a person's notification choices: which kinds of
// message reach them on the bell and by email. The auth plugin stores them on
// the user row; plugins that send messages ask through a small interface of
// their own and never read the row themselves.
//
// Safety mail (password resets, email changes, ownership hand-overs, campaign
// invites) is not a category here, so it can never be switched off.
package notifyprefs

import (
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Channel is how a message reaches someone.
type Channel string

const (
	Bell  Channel = "bell"
	Email Channel = "email"
)

// Category keys. They are the wire values the account page sends and the keys
// of the stored JSON, so renaming one loses people's stored choices.
const (
	GameNightInvites = "gameNightInvites"
	GameNightChanges = "gameNightChanges"
	GameNightAnswers = "gameNightAnswers"
	AvailabilityAsks = "availabilityAsks"
	ItemsGiven       = "itemsGiven"
	ModuleUpdates    = "moduleUpdates"
)

// Category is one row of the account page's table. A channel the category
// has no message for is left out of Channels, so the page shows a dash and a
// save for it is refused.
type Category struct {
	Key      string
	Group    string
	Label    string
	Detail   string
	Channels []Channel
	// Defaults is what a person gets before choosing; email is opt-in
	// except for the messages people expect in their inbox.
	Defaults map[Channel]bool
}

// Catalog lists every category in page order.
var Catalog = []Category{
	{Key: GameNightInvites, Group: "Game nights", Label: "A game night is planned or a time is suggested",
		Detail:   "Includes the invitation email with Going / Can't make it buttons",
		Channels: []Channel{Bell, Email}, Defaults: map[Channel]bool{Bell: true, Email: true}},
	{Key: GameNightChanges, Group: "Game nights", Label: "A game night moves, is cancelled or comes back",
		Channels: []Channel{Bell}, Defaults: map[Channel]bool{Bell: true}},
	{Key: GameNightAnswers, Group: "Game nights", Label: "Someone answers a time I suggested, or suggests another time",
		Channels: []Channel{Bell}, Defaults: map[Channel]bool{Bell: true}},
	{Key: AvailabilityAsks, Group: "Game nights", Label: "The owner asks me to fill in or confirm my free times",
		Channels: []Channel{Bell, Email}, Defaults: map[Channel]bool{Bell: true, Email: false}},
	{Key: ItemsGiven, Group: "Items and campaigns", Label: "Someone gives my character an item",
		Channels: []Channel{Bell}, Defaults: map[Channel]bool{Bell: true}},
	{Key: ModuleUpdates, Group: "Items and campaigns", Label: "A new Foundry module version is out",
		Detail:   "Campaign owners only",
		Channels: []Channel{Email}, Defaults: map[Channel]bool{Email: true}},
}

// bellTypes maps each bell notification type a plugin writes to its
// category. A type missing here is always delivered, so a new kind of
// message is never silently dropped before it gets a row on the page.
var bellTypes = map[string]string{
	"proposal_created":             GameNightInvites,
	"proposal_confirmed":           GameNightInvites,
	"session_moved":                GameNightChanges,
	"session_cancelled":            GameNightChanges,
	"session_restored":             GameNightChanges,
	"proposal_response":            GameNightAnswers,
	"session_reschedule_suggested": GameNightAnswers,
	"availability_nudge":           AvailabilityAsks,
	"availability_confirm":         AvailabilityAsks,
	"item_given":                   ItemsGiven,
}

// CategoryForBellType returns the category of a bell notification type, or
// "" when the type has none and is always delivered.
func CategoryForBellType(ntype string) string { return bellTypes[ntype] }

func find(key string) (Category, bool) {
	for _, c := range Catalog {
		if c.Key == key {
			return c, true
		}
	}
	return Category{}, false
}

// Has reports whether the category sends anything on that channel.
func (c Category) Has(ch Channel) bool {
	for _, x := range c.Channels {
		if x == ch {
			return true
		}
	}
	return false
}

// Prefs is one person's choices with defaults filled in.
type Prefs struct {
	// Choices maps category key to channel to on/off.
	Choices map[string]map[Channel]bool `json:"choices"`
	// PauseEmail stops every email in the catalog without losing the
	// per-row choices, so un-pausing restores them.
	PauseEmail bool `json:"pauseEmail"`
}

// Default returns the choices a person has before changing anything.
func Default() Prefs {
	p := Prefs{Choices: map[string]map[Channel]bool{}}
	for _, c := range Catalog {
		row := map[Channel]bool{}
		for _, ch := range c.Channels {
			row[ch] = c.Defaults[ch]
		}
		p.Choices[c.Key] = row
	}
	return p
}

// Allows reports whether a message of this category may go out on this
// channel. An unknown category is allowed: the catalog only ever narrows.
func (p Prefs) Allows(category string, ch Channel) bool {
	c, ok := find(category)
	if !ok || !c.Has(ch) {
		return true
	}
	if ch == Email && p.PauseEmail {
		return false
	}
	return p.Choices[category][ch]
}

// Parse reads the stored JSON. Missing or unknown entries fall back to the
// defaults one by one, so a malformed row degrades to the default choices
// rather than failing a send or the page.
func Parse(raw []byte) Prefs {
	out := Default()
	if len(raw) == 0 {
		return out
	}
	var stored struct {
		Choices    map[string]map[Channel]*bool `json:"choices"`
		PauseEmail bool                         `json:"pauseEmail"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return out
	}
	out.PauseEmail = stored.PauseEmail
	for key, row := range stored.Choices {
		c, ok := find(key)
		if !ok {
			continue
		}
		for ch, v := range row {
			if v != nil && c.Has(ch) {
				out.Choices[key][ch] = *v
			}
		}
	}
	return out
}

// Update is a PARTIAL change: each switch saves on its own as it is pressed,
// so only the entries present change and a second device's choices survive.
type Update struct {
	Choices    map[string]map[Channel]bool `json:"choices"`
	PauseEmail *bool                       `json:"pauseEmail"`
}

// ApplyTo returns cur with the update merged in, refusing a category or
// channel the catalog doesn't have.
func (u Update) ApplyTo(cur Prefs) (Prefs, error) {
	out := Prefs{Choices: map[string]map[Channel]bool{}, PauseEmail: cur.PauseEmail}
	for k, row := range cur.Choices {
		cp := map[Channel]bool{}
		for ch, v := range row {
			cp[ch] = v
		}
		out.Choices[k] = cp
	}
	for key, row := range u.Choices {
		c, ok := find(key)
		if !ok {
			return cur, apperror.NewBadRequest("unknown notification kind")
		}
		for ch, v := range row {
			if !c.Has(ch) {
				return cur, apperror.NewBadRequest("that notification isn't sent that way")
			}
			out.Choices[key][ch] = v
		}
	}
	if u.PauseEmail != nil {
		out.PauseEmail = *u.PauseEmail
	}
	return out, nil
}
