package quests

import (
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Viewer is who is asking, reduced to plain values so services never see
// Echo or campaign types. The handler builds it from the campaign context.
type Viewer struct {
	UserID string
	// MemberRole is the real membership role (0 guest .. 3 owner); it decides
	// who may change a board.
	MemberRole int
	// VisibilityRole is the role used for page visibility; a co-DM counts as
	// an owner there.
	VisibilityRole int
	// IsDM is the owner or a member with DM access.
	IsDM bool
}

// IsPlayer reports a real member (a guest of a public campaign is not).
func (v Viewer) IsPlayer() bool { return v.MemberRole >= permissions.RolePlayer || v.IsDM }

// IsScribe reports scribe or above.
func (v Viewer) IsScribe() bool { return v.MemberRole >= permissions.RoleScribe || v.IsDM }

// Allowed enums.
const (
	StatusNotStarted = "not_started"
	StatusActive     = "active"
	StatusDone       = "done"
	StatusFailed     = "failed"

	LookLit       = "lit"
	LookPlain     = "plain"
	LookParchment = "parchment"
	LookMidnight  = "midnight"

	WhoDM     = "dm"
	WhoScribe = "scribe"
	WhoAll    = "all"

	KindNotice = "notice"
	KindNote   = "note"
	KindPage   = "page"
	KindMap    = "map"
	KindString = "string"
)

// --- Quest document (stored as JSON in quests.data) ---

// Notice is the posting on the board. Plain text only: the widgets render it
// as text, never HTML.
type Notice struct {
	Plate    string   `json:"plate"`
	Kicker   string   `json:"kicker"`
	Title    string   `json:"title"`
	Blurb    string   `json:"blurb"`
	Body     []string `json:"body"`
	PostedBy string   `json:"postedBy"`
	Reward   string   `json:"reward"`
	Due      string   `json:"due"`
}

// Step is one ledger step; only shown steps reach players.
type Step struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Done  bool   `json:"done"`
	Shown bool   `json:"shown"`
}

// Reward is a DM-only ledger entry (the player-facing line is Notice.Reward).
type Reward struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Text     string   `json:"text"`
	Amount   *float64 `json:"amount"`
	EntityID string   `json:"entityId"`
}

// Foe is a DM-only ledger entry.
type Foe struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Note     string `json:"note"`
	EntityID string `json:"entityId"`
}

// Link is a DM-only ledger link to a page or a map.
type Link struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	RefID string `json:"refId"`
	Label string `json:"label"`
}

// Rect places a piece on the cork board, in percent of the board.
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w"`
	R      float64 `json:"r"`
	Hidden bool    `json:"hidden"`
}

// Layout holds the three pieces of the quest board.
type Layout struct {
	Notice Rect `json:"notice"`
	Map    Rect `json:"map"`
	Tag    Rect `json:"tag"`
}

// LayoutPatch replaces whichever pieces it carries and keeps the rest.
type LayoutPatch struct {
	Notice *Rect `json:"notice"`
	Map    *Rect `json:"map"`
	Tag    *Rect `json:"tag"`
}

// Looks are the board and ledger themes.
type Looks struct {
	Board  string `json:"board"`
	Ledger string `json:"ledger"`
}

// Quest is the whole stored document.
type Quest struct {
	Notice    Notice   `json:"notice"`
	Status    string   `json:"status"`
	HandedOut bool     `json:"handedOut"`
	Steps     []Step   `json:"steps"`
	Rewards   []Reward `json:"rewards"`
	Foes      []Foe    `json:"foes"`
	Links     []Link   `json:"links"`
	MapID     string   `json:"mapId"`
	Layout    Layout   `json:"layout"`
	Looks     Looks    `json:"looks"`
	// DueDate is a day on the campaign calendar; nil means none. DueEventID
	// is the calendar event that mirrors it, so a later change can update
	// that event instead of adding another.
	DueDate    *DueDay `json:"dueDate,omitempty"`
	DueEventID string  `json:"dueEventId,omitempty"`
}

// QuestPatch is the PUT body. Absent key preserves, present replaces; an
// explicit null clears mapId and dueDate (every other field has no "cleared"
// state and preserves, as patch.Field.Val does). Version is required: it is the
// version the editor loaded.
type QuestPatch struct {
	Version   patch.Field[int]         `json:"version"`
	Notice    patch.Field[Notice]      `json:"notice"`
	Status    patch.Field[string]      `json:"status"`
	HandedOut patch.Field[bool]        `json:"handedOut"`
	Steps     patch.Field[[]Step]      `json:"steps"`
	Rewards   patch.Field[[]Reward]    `json:"rewards"`
	Foes      patch.Field[[]Foe]       `json:"foes"`
	Links     patch.Field[[]Link]      `json:"links"`
	MapID     patch.Field[string]      `json:"mapId"`
	Layout    patch.Field[LayoutPatch] `json:"layout"`
	Looks     patch.Field[LooksPatch]  `json:"looks"`
	DueDate   patch.Field[DueDay]      `json:"dueDate"`
}

// --- Quest responses ---

// RewardView is a Reward with the linked page's name resolved.
type RewardView struct {
	Reward
	Name string `json:"name"`
}

// FoeView is a Foe with the linked page's name resolved.
type FoeView struct {
	Foe
	Name string `json:"name"`
}

// LinkView is a Link with its target's name resolved.
type LinkView struct {
	Link
	Name string `json:"name"`
}

// MapRef is the map scrap's target.
type MapRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DayView is a calendar day with its label in the calendar's own style.
type DayView struct {
	Year  int    `json:"year"`
	Month int    `json:"month"`
	Day   int    `json:"day"`
	Label string `json:"label"`
}

// DueView is a quest's due date with the days left; negative when late.
type DueView struct {
	DayView
	DaysLeft int `json:"daysLeft"`
}

// CalendarView is what the DM's due-date picker needs to draw the calendar.
type CalendarView struct {
	Name       string          `json:"name"`
	Today      DayView         `json:"today"`
	Months     []CalendarMonth `json:"months"`
	LeapEvery  int             `json:"leapEvery"`
	LeapOffset int             `json:"leapOffset"`
}

// DMQuestView is everything, for the DM team.
type DMQuestView struct {
	CanEdit   bool         `json:"canEdit"`
	Version   int          `json:"version"`
	Notice    Notice       `json:"notice"`
	Status    string       `json:"status"`
	HandedOut bool         `json:"handedOut"`
	Steps     []Step       `json:"steps"`
	Rewards   []RewardView `json:"rewards"`
	Foes      []FoeView    `json:"foes"`
	Links     []LinkView   `json:"links"`
	MapID     string       `json:"mapId"`
	MapName   string       `json:"mapName"`
	Layout    Layout       `json:"layout"`
	Looks     Looks        `json:"looks"`
	// MapsOn is false when the maps addon is off, so map choices are hidden.
	MapsOn bool `json:"mapsOn"`
	// Calendar is null when the campaign has no usable calendar; the DM
	// view alone carries it. Due is null without a due date or a calendar.
	Calendar *CalendarView `json:"calendar"`
	Due      *DueView      `json:"due"`
}

// PlayerStep is a shown step without its id.
type PlayerStep struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// PlayerQuestView is all a non-DM viewer receives: no reward list, foes,
// links, hidden steps or the ids of anything hidden. Notice and Map are null
// when the DM hid that piece.
type PlayerQuestView struct {
	CanEdit     bool         `json:"canEdit"`
	Notice      *Notice      `json:"notice"`
	Status      string       `json:"status"`
	HandedOut   bool         `json:"handedOut"`
	Steps       []PlayerStep `json:"steps"`
	HiddenSteps bool         `json:"hiddenSteps"`
	ShowTag     bool         `json:"showTag"`
	Layout      Layout       `json:"layout"`
	Looks       Looks        `json:"looks"`
	Map         *MapRef      `json:"map"`
	// Due is null without a due date, or while the DM hides the notice.
	Due *DueView `json:"due"`
}

// --- Boards ---

// Home is where a set of boards lives: on one page (EntityID) or on a
// category's dashboard (TypeID, an entity type). Exactly one is set; the
// repository refuses anything else, so a mix-up cannot widen a query.
type Home struct {
	EntityID string
	TypeID   int
}

// PageHome is the home of a place page's boards.
func PageHome(entityID string) Home { return Home{EntityID: entityID} }

// TypeHome is the home of a category's boards.
func TypeHome(typeID int) Home { return Home{TypeID: typeID} }

// IsType reports whether the home is a category.
func (h Home) IsType() bool { return h.TypeID != 0 }

// Board is a stored board.
type Board struct {
	ID         string
	CampaignID string
	Home       Home
	Name       string
	Who        string
	SortOrder  int
}

// Item is a stored board item. Empty strings stand for NULL.
type Item struct {
	ID          string
	BoardID     string
	CampaignID  string
	Kind        string
	X, Y, W, R  float64
	OwnerUserID string
	// ByDM records the poster's DM capability at creation, so a later role
	// change cannot turn a player's pin into a DM piece or the reverse.
	ByDM   bool
	Hidden bool
	Text   string
	RefID  string
	FromID string
	ToID   string
}

// ItemInput creates an item.
type ItemInput struct {
	Kind  string  `json:"kind"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	W     float64 `json:"w"`
	R     float64 `json:"r"`
	Text  string  `json:"text"`
	RefID string  `json:"refId"`
	From  string  `json:"from"`
	To    string  `json:"to"`
}

// ItemPatch moves or edits an item; absent preserves. Text and Hidden have no
// cleared state, so null preserves them.
type ItemPatch struct {
	X      patch.Field[float64] `json:"x"`
	Y      patch.Field[float64] `json:"y"`
	W      patch.Field[float64] `json:"w"`
	R      patch.Field[float64] `json:"r"`
	Text   patch.Field[string]  `json:"text"`
	Hidden patch.Field[bool]    `json:"hidden"`
}

// BoardPatch renames a board or changes who may pin to it.
type BoardPatch struct {
	Name patch.Field[string] `json:"name"`
	Who  patch.Field[string] `json:"who"`
}

// LooksPatch changes a page's board and ledger themes.
type LooksPatch struct {
	Board  patch.Field[string] `json:"board"`
	Ledger patch.Field[string] `json:"ledger"`
}

// ItemView is one item as a viewer sees it. One flat shape for every kind;
// the fields of other kinds are omitted. Concealed pages carry no name or id.
type ItemView struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	W            float64 `json:"w"`
	R            float64 `json:"r"`
	Mine         bool    `json:"mine"`
	ByDM         bool    `json:"byDm"`
	OwnerName    string  `json:"ownerName"`
	OwnerInitial string  `json:"ownerInitial"`
	Hidden       *bool   `json:"hidden,omitempty"` // DM only

	// notice
	QuestID  string `json:"questId,omitempty"`
	Title    string `json:"title,omitempty"`
	Kicker   string `json:"kicker,omitempty"`
	Blurb    string `json:"blurb,omitempty"`
	Reward   string `json:"reward,omitempty"`
	Status   string `json:"status,omitempty"`
	HasSheet bool   `json:"hasSheet,omitempty"`
	// DaysLeft is a pointer so that 0 (due today) is not dropped; it is
	// absent when the quest has no due date.
	DaysLeft *int `json:"daysLeft,omitempty"`

	// page / map
	EntityID  string `json:"entityId,omitempty"`
	MapID     string `json:"mapId,omitempty"`
	Name      string `json:"name,omitempty"`
	TypeName  string `json:"typeName,omitempty"`
	URL       string `json:"url,omitempty"`
	Concealed bool   `json:"concealed,omitempty"`
	// ImagePath is resolved to ImageURL by the handler, which owns the signed
	// media URL helpers.
	ImagePath string `json:"-"`
	ImageURL  string `json:"imageUrl,omitempty"`

	// note
	Text string `json:"text,omitempty"`

	// string
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// BoardSummary is a board without its items.
type BoardSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Who       string `json:"who"`
	CanChange bool   `json:"canChange"`
}

// BoardView is a board with the items the viewer may see.
type BoardView struct {
	BoardSummary
	Items []ItemView `json:"items"`
}

// Me tells the widget who is looking, so it need not guess from the page.
type Me struct {
	UserID string `json:"userId"`
	IsDM   bool   `json:"isDm"`
	Role   int    `json:"role"`
}

// BoardsView is the GET response for a place page.
type BoardsView struct {
	CanManage bool        `json:"canManage"`
	Me        Me          `json:"me"`
	Looks     Looks       `json:"looks"`
	Boards    []BoardView `json:"boards"`
	// MapsOn is false when the maps addon is off, so map choices are hidden.
	MapsOn bool `json:"mapsOn"`
}

// PickerItem is one search hit for the pinning picker; maps carry only id
// and name.
type PickerItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	TypeName  string `json:"typeName,omitempty"`
	Player    string `json:"player,omitempty"`
	ImagePath string `json:"-"`
	ImageURL  string `json:"imageUrl,omitempty"`
}
