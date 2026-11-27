// desk_model.go holds the types behind Campaign settings (the owner's desk):
// the facts other plugins supply, the provider seam they register through,
// and the rows and values the pages draw. A row can only say its name and its
// values, never a phrase. Rows are built in desk_rows.go, the Overview's tiles
// in desk_board.go, and both are assembled in desk_service.go, so everything
// is table-testable without a database, a socket or an Echo context.

package campaigns

import (
	"context"
	"strings"
	"time"
)

// DeskSlot names one line of the desk that belongs to another plugin.
type DeskSlot string

// The slots a plugin can fill. A slot nobody fills, or whose provider fails,
// is drawn as the quiet word "Open" with no figure.
const (
	DeskChanges DeskSlot = "changes" // what players changed
	DeskMedia   DeskSlot = "media"   // pictures and files
	DeskFoundry DeskSlot = "foundry" // the Foundry connection
	DeskAI      DeskSlot = "ai"      // the owner's AI connection
	DeskTrash   DeskSlot = "trash"   // pages waiting in the campaign Trash
	DeskKeys    DeskSlot = "keys"    // the campaign's API keys
	DeskEntries DeskSlot = "entries" // the campaign's own rules entries
)

// DeskRequest says whose desk is being drawn. Providers read it, never an
// Echo context, so they stay plain services. The route is owner-only; a
// provider may rely on that and must still read only this campaign.
type DeskRequest struct {
	CampaignID string
	OwnerID    string
	Now        time.Time
}

// DeskProvider is how another plugin tells the desk one line's numbers, the
// same inversion as the settings-tab registry: campaigns never imports the
// plugin, the plugin registers itself. Facts returns only the pointer for its
// own slot.
type DeskProvider interface {
	// Slot is the one line this provider fills.
	Slot() DeskSlot
	// Facts reads that line's live numbers. An error degrades the line to
	// "Open" and is logged; it never fails the page.
	Facts(ctx context.Context, req DeskRequest) (DeskFacts, error)
}

// DeskFacts is the bag of numbers providers fill. Each pointer belongs to one
// slot; nil means the line has no numbers this time.
type DeskFacts struct {
	Changes *DeskChangesFacts
	Media   *DeskMediaFacts
	Foundry *DeskFoundryFacts
	AI      *DeskAIFacts
	Trash   *DeskTrashFacts
	Keys    *DeskKeysFacts
	Entries *DeskEntriesFacts
}

// DeskEntriesFacts counts the campaign's own rules entries and what waits on
// a Director in those lists.
type DeskEntriesFacts struct {
	Entries int // accepted entries, not drafts
	Drafts  int // drafts a player has handed in
	Asks    int // Ask-first picks a player is waiting on
}

// Waiting is everything in the lists that waits for a Director.
func (f DeskEntriesFacts) Waiting() int { return f.Drafts + f.Asks }

// DeskChangesFacts counts what players changed in the last week.
type DeskChangesFacts struct {
	Changes int // changes recorded in the last 7 days
	Players int // how many different players made them
}

// DeskMediaFacts is what the campaign's picture store holds.
type DeskMediaFacts struct {
	Files int
	Bytes int64
}

// DeskFoundryFacts is the campaign's Foundry link as the connector reports it.
type DeskFoundryFacts struct {
	Connection FoundryConnection
}

// DeskAIState is where the owner's AI connection stands.
type DeskAIState string

// The states of the owner's AI connection.
const (
	DeskAINone      DeskAIState = "none"
	DeskAIConnected DeskAIState = "connected"
	DeskAIPaused    DeskAIState = "paused"
	DeskAIEnded     DeskAIState = "ended"
)

// DeskAIFacts is the owner's AI connection and what it has waiting.
type DeskAIFacts struct {
	State  DeskAIState
	Client string // what the AI app called itself; empty when it never said
	CanAsk bool   // may ask for changes; false is "only look"
	// Waiting is how many of its requests wait for the owner's answer.
	Waiting int
}

// DeskTrashFacts is what waits in the campaign Trash.
type DeskTrashFacts struct {
	Items int // pages, pictures, game nights and maps
	Days  int // how long each waits before it is gone for good
}

// DeskKeysFacts is how many API keys the campaign has in use.
type DeskKeysFacts struct {
	Count int
}

// DeskInputs is everything campaigns itself knows that the desk draws: the
// campaign, its members and the game and features read through the existing
// listers. Gathered by the handler so the service stays free of Echo.
type DeskInputs struct {
	Campaign *Campaign
	Members  []CampaignMember
	// SystemID is the campaign's current game system id; empty when none.
	SystemID string
	// SystemName is the chosen game system's name; empty when none is chosen
	// or the system list does not name it.
	SystemName string
	// FeaturesOn and FeaturesTotal count the installed features; both zero
	// when the add-on list could not be read.
	FeaturesOn, FeaturesTotal int
	// Features is the add-on list itself, for the board's chips; nil when it
	// could not be read.
	Features []PluginHubAddon
	// Addons says which add-ons are on for this campaign: the Foundry tile
	// and the storage figures exist only while theirs is.
	Addons map[string]bool
	// Invited is how many invitations still wait for an answer; InvitedRead
	// says they could be counted, so an unread count is never drawn as zero.
	Invited     int
	InvitedRead bool
	// GMTerm is the campaign's own word for its game master, for the words a
	// service builds (the template reads it from its context instead).
	GMTerm string
	// Look is the saved look in words, for the Look tile and the Campaign
	// tab's Look row.
	Look LookSummary
	// Categories are the campaign's categories; CategoriesRead says they
	// could be read, so an unread list is never drawn as none.
	Categories     []CustomizeCategory
	CategoriesRead bool
	// Tabs are the areas other plugins add to Customize, in Customize's
	// order, with their add-on on, each with what it says is chosen now.
	Tabs []DeskTab
	// BrushHidden is the viewer's own choice to hide the header's Customize
	// brush in this campaign. It is the person's, not the campaign's.
	BrushHidden bool
}

// DeskTab is one plugin-contributed Customize area as its row needs it.
type DeskTab struct {
	ID, Label string
	Summary   CustomizeSummary
	// Blurb says what the area holds, for a plugin that gives no Summary.
	Blurb string
}

// Desk is the Overview page: the board of tiles.
type Desk struct {
	CampaignName string
	Board        DeskBoard
}

// DeskLine is one setting: its name, and the values that stand beside it.
type DeskLine struct {
	ID string
	// Name is what the row is called, in a few words.
	Name string
	// Values stand beside the name: one value, or several quiet words.
	Values []DeskValue
	// Why is the reason a value of this row can be off, drawn under the row
	// while one is. A blank carries its own reason (an archived campaign), so
	// this is only for a row whose off state can change on the page.
	Why string
}

// DeskValue is what stands beside a row's name. It is one of four things: a
// value that changes where it stands (Blank), a figure that opens its page
// (Href), a quiet word that acts or leads on (Word, with View or Href), or a
// figure with a state dot. It never holds a phrase.
type DeskValue struct {
	// Text is the figure or the word; Sub is a muted second part ("this
	// week", "310 MB").
	Text, Sub string
	// Href makes the value a link; unless Word is set it ends in a chevron.
	Href string
	// Word is a quiet word: text weight, no chevron.
	Word bool
	// State is the dot before the value: "", "ok" or "warn". State colours
	// mean state only.
	State string
	// Swatch draws the campaign's colour as a dot before the value.
	Swatch bool
	// SubSwatch draws the campaign's colour as a dot at the start of the
	// muted part, the colour's name standing beside it.
	SubSwatch bool
	// Joined puts a middle dot between the figure and its muted part, which
	// then reads as more of the same answer ("8 · 1 hidden from players").
	Joined bool
	// Danger draws the word in the state red: only Delete.
	Danger bool
	// Blank is set for a value that is changed where it stands.
	Blank *DeskBlank
	// View is set for a word that opens the campaign as another kind of
	// viewer, by posting the view-as form.
	View *DeskView
	// Off is why the value cannot be used right now ("" when it can).
	Off string
	// NeedsPublic marks the guest preview, which is off exactly while the
	// campaign is private, so the page can enable it the moment the campaign
	// is made public.
	NeedsPublic bool
}

// DeskView is the form behind "As a player" and "As a guest": it posts to the
// view-as route with who to look as and where to land.
type DeskView struct {
	As   string // "player" or "guest"
	Next string // "home"
}

// DeskFigure is what a Manage tool page draws beside its title: muted
// figures joined by a middle dot, with an optional state dot before them.
// It is built beside the desk's rows from the same facts, so the page and
// the row cannot disagree. The zero value draws nothing.
type DeskFigure struct {
	// State is the dot before the figures: "", "ok" or "warn".
	State string
	Parts []string
}

// Empty reports whether there is nothing to draw.
func (f DeskFigure) Empty() bool { return len(f.Parts) == 0 }

// Text is the figures joined with a middle dot.
func (f DeskFigure) Text() string { return strings.Join(f.Parts, " · ") }

// DeskBlankKind is how a blank is changed.
type DeskBlankKind string

// The ways a blank changes: a small menu of choices, typing a one-line name,
// or typing a longer text in a field of a few lines.
const (
	DeskBlankChoice DeskBlankKind = "choice"
	DeskBlankName   DeskBlankKind = "name"
	DeskBlankText   DeskBlankKind = "text"
)

// DeskBlank is a row's value that saves the moment it is changed, through the
// existing route for that setting.
type DeskBlank struct {
	Kind DeskBlankKind
	// ID is stable for tests and for the page's own scripts.
	ID string
	// Label names what the blank sets, for the screen reader and the menu.
	Label string
	// Value is the stored value; Shown is the words on screen for it.
	Value, Shown string
	// Empty draws Shown (the word "None") in muted ink: there is no value.
	Empty   bool
	Options []DeskOption
	// URL, Method and Field say where a change goes: a JSON body of
	// {Field: value} to URL. Bool sends the value as true or false instead
	// of text, for the settings that are flags.
	URL, Method, Field string
	Bool               bool
	// Max is the longest text the route accepts; zero for a choice.
	Max int
	// Off is why the blank cannot be changed right now ("" when it can): an
	// archived campaign is read-only, and its writes are refused.
	Off string
}

// DeskOption is one choice in a blank's menu.
type DeskOption struct {
	Value string `json:"v"`
	Label string `json:"l"`
	// Hint is a muted second line under the label.
	Hint string `json:"h,omitempty"`
}
