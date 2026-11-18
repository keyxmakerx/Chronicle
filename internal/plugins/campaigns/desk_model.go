// desk_model.go holds the types of the owner's desk (Manage's home page): the
// facts other plugins supply, the provider seam they register through, and
// the page model the templates draw. Wording lives in desk_sentences.go and
// the assembly in desk_service.go, so every sentence is table-testable
// without a database, a socket or an Echo context.

package campaigns

import (
	"context"
	"time"
)

// DeskSlot names one line of the desk that belongs to another plugin.
type DeskSlot string

// The slots a plugin can fill. A slot nobody fills, or whose provider fails,
// is drawn as a plain link sentence with no number.
const (
	DeskChanges DeskSlot = "changes" // what players changed
	DeskMedia   DeskSlot = "media"   // pictures and files
	DeskFoundry DeskSlot = "foundry" // the Foundry connection
	DeskAI      DeskSlot = "ai"      // the owner's AI connection
	DeskTrash   DeskSlot = "trash"   // pages waiting in the campaign Trash
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
	// Facts reads that line's live numbers. An error degrades the line to a
	// plain sentence and is logged; it never fails the page.
	Facts(ctx context.Context, req DeskRequest) (DeskFacts, error)
}

// DeskFolder is the optional second half of a provider: the few things people
// come for when a line unfolds, read only when the owner opens it so the desk
// itself stays quick. A provider without it leaves its line's fold as a plain
// sentence and the link to the full page.
type DeskFolder interface {
	// Fold reads what the line's sub menu shows. An error is logged and
	// shows the plain fallback, never a broken fold.
	Fold(ctx context.Context, req DeskRequest) (*DeskFold, error)
}

// DeskFacts is the bag of numbers providers fill. Each pointer belongs to one
// slot; nil means the line has no numbers this time.
type DeskFacts struct {
	Changes *DeskChangesFacts
	Media   *DeskMediaFacts
	Foundry *DeskFoundryFacts
	AI      *DeskAIFacts
	Trash   *DeskTrashFacts
}

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

// DeskInputs is everything campaigns itself knows that the desk draws: the
// campaign, its members and the game and features read through the existing
// listers. Gathered by the handler so the service stays free of Echo.
type DeskInputs struct {
	Campaign *Campaign
	Members  []CampaignMember
	// SystemName is the chosen game system's name; empty when none is chosen.
	SystemName string
	// FeaturesOn and FeaturesTotal count the installed features; both zero
	// when the add-on list could not be read.
	FeaturesOn, FeaturesTotal int
	// Addons says which add-ons are on for this campaign: lines for Media and
	// Foundry exist only while theirs is.
	Addons map[string]bool
	// Invited is how many invitations still wait for an answer.
	Invited int
	// GMTerm is the campaign's own word for its game master, for the words a
	// service builds (the template reads it from its context instead).
	GMTerm string
}

// Desk is the page: plain sentences in groups.
type Desk struct {
	CampaignName string
	Needs        []NeedsYouItem
	Groups       []DeskGroup
}

// DeskGroup is a named run of lines, drawn under a quiet label.
type DeskGroup struct {
	Name  string
	Lines []DeskLine
}

// DeskLine is one sentence with the quiet words that open its tool.
type DeskLine struct {
	ID string
	// Fold says the line unfolds into a sub menu; its content is fetched when
	// the owner first opens it.
	Fold bool
	// State is the dot before the sentence: "", "ok" or "warn". State colours
	// mean state only.
	State string
	Parts []DeskPart
	Acts  []DeskAct
}

// DeskPart is a piece of a sentence: plain words, bold words, or a blank the
// owner changes in place.
type DeskPart struct {
	Text string
	Bold bool
	// GM draws the campaign's own word for its game master (GMTerm), which
	// only the template can read from its context.
	GM bool
	// After marks the words that follow a blank and change with its value;
	// the page rewrites them in place when the blank is changed.
	After bool
	// Swatch draws the campaign's colour as a small dot (the Customize line).
	Swatch bool
	Blank  *DeskBlank
}

// DeskAct is a quiet word that leads to a tool's own page.
type DeskAct struct {
	Label string
	Href  string
	// Danger draws the word in the state red: only Delete.
	Danger bool
}

// DeskBlankKind is how a blank is changed.
type DeskBlankKind string

// The ways a blank changes: a small menu of choices, or typing a name.
const (
	DeskBlankChoice DeskBlankKind = "choice"
	DeskBlankName   DeskBlankKind = "name"
)

// DeskBlank is a dashed word that saves the moment it is changed, through the
// existing route for that setting.
type DeskBlank struct {
	Kind DeskBlankKind
	// ID is stable for tests and for the page's own scripts.
	ID string
	// Label names what the blank sets, for the screen reader and the menu.
	Label string
	// Value is the stored value; Shown is the words on screen for it.
	Value, Shown string
	Options      []DeskOption
	// URL, Method and Field say where a change goes: a JSON body of
	// {Field: value} to URL. Bool sends the value as true or false instead
	// of text, for the settings that are flags.
	URL, Method, Field string
	Bool               bool
	// Expect says how the save answers: "" is JSON with an error status, and
	// "redirect" is the People role route, which answers a redirect when it
	// worked and a page with the reason when it did not.
	Expect string
	// Local says the choice is kept on the page and saved by something else
	// (the invite's role): picking it sends nothing.
	Local bool
	// Off is why the blank cannot be changed right now ("" when it can): an
	// archived campaign is read-only, and its writes are refused.
	Off string
}

// DeskOption is one choice in a blank's menu.
type DeskOption struct {
	Value string `json:"v"`
	Label string `json:"l"`
	// After replaces the words that follow the blank once this is chosen, for
	// a sentence whose ending depends on the value.
	After string `json:"a,omitempty"`
}

// DeskFold is what a line shows when it unfolds: a few rows and the one
// thing people most often do there, with the full page still one quiet word
// away on the line itself.
type DeskFold struct {
	// Note is a plain sentence above the rows ("why it stopped"). Also the
	// whole fold when a provider fails.
	Note string
	Rows []DeskFoldRow
	// More is how many rows are left out, drawn as "and N more" leading to
	// MoreHref, the full page.
	More     int
	MoreHref string
	// Invite is the invite field, drawn only on the People line.
	Invite *DeskInvite
	// Fallback marks a fold that could not be read: it shows Note and a link.
	Fallback bool
	// FullHref and FullLabel are the link under a fallback sentence.
	FullHref, FullLabel string
}

// DeskFoldRow is one row of a fold: who or what, a quiet detail, and either a
// dashed word that changes in place or one action.
type DeskFoldRow struct {
	// Initial is the letter in the round mark; empty draws none.
	Initial string
	Lead    string
	Text    string
	Meta    string
	Blank   *DeskBlank
	Act     *DeskFoldAct
}

// DeskFoldAct is one existing write route a row calls, with the words for it.
// The page only calls routes that already check the owner; this never names a
// new one.
type DeskFoldAct struct {
	Label  string
	URL    string
	Method string
	// Busy is what the word says while the call runs.
	Busy string
}

// DeskInvite is the invite field of the People fold: it posts to the existing
// invite route.
type DeskInvite struct {
	URL string
	// Roles are the choices offered for the invited person.
	Roles []DeskOption
}
