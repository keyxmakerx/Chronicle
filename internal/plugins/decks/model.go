package decks

import (
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// Deck kinds. A kind only sets defaults; every deck is the same thing with
// settings.
const (
	KindHero  = "hero"
	KindParty = "party"
	KindEvent = "event"
)

// Zones a copy can be in.
const (
	ZonePile  = "pile"
	ZoneShown = "shown"
	ZoneHand  = "hand"
	ZonePlay  = "play"
	ZoneUsed  = "used"
	ZoneGone  = "gone"
)

// Fates: where a copy goes after it is played, taken off or discarded.
const (
	FateBack = "back"
	FateUsed = "used"
	FateGone = "gone"
	FatePlay = "play"
)

// Log actions. The names are stable and plain on purpose: later they become
// the triggers a rule can listen for, so changing one is a change to those.
const (
	ActDraw     = "drawn"
	ActDeal     = "dealt"
	ActTurn     = "turned"
	ActShow     = "shown"
	ActPlay     = "played"
	ActAsk      = "asked"
	ActApprove  = "approved"
	ActTurnDown = "turned-down"
	ActTakeOff  = "taken-off"
	ActDiscard  = "discarded"
	ActTakeBack = "taken-back"
	ActPutBack  = "put-back"
	ActKeep     = "kept"
	ActShuffle  = "shuffled"
	ActGather   = "gathered"
)

// Limits. They bound what a Director can store; the service enforces them.
const (
	MaxDecksPerCampaign = 60
	MaxCardsPerDeck     = 200
	MaxCopiesPerDeck    = 600
	MaxCopiesPerCard    = 20
	MaxNameChars        = 120
	MaxWordsChars       = 2000
	MaxNoteChars        = 2000
	MaxKindKeyChars     = 32
	MaxCornerChars      = 12
	MaxPictureChars     = 255
	MaxBackWordsChars   = 24
	MaxKinds            = 12
	// MaxShown is how many copies of one deck the shown spot holds at once.
	MaxShown = 5
	// MaxDrawCount is how many cards a Director takes in one draw.
	MaxDrawCount     = 5
	MaxKindNameChars = 24
	MaxPlayersChosen = 200
	MaxDraws         = 1000
	LogKept          = 1000
	LogShown         = 200
	MaxBodyBytes     = 256 << 10
	maxIDChars       = 64
)

// Viewer is who is asking, taken from the campaign context in the handler.
type Viewer struct {
	UserID string
	// MemberRole is the real membership role (0 none/guest .. 3 owner). A
	// player rule needs at least RolePlayer, so a former member or a visitor
	// to a public campaign never matches.
	MemberRole int
	// VisibilityRole decides what the viewer may see on a hero page.
	VisibilityRole int
	// Director is the owner or a co-Director.
	Director bool
}

// Deck is a stored deck.
type Deck struct {
	ID             string
	CampaignID     string
	Name           string
	Kind           string
	HolderEntityID string
	// NextCardID and LastCardID are the Director's two nudges to the draw: the
	// card the next draw takes, and the card left out while anything else is in
	// the pile. "" is unset. Players never receive either.
	NextCardID string
	LastCardID string
	Settings   Settings
	Look       Look
	Version    int
	SortOrder  int
	CreatedAt  time.Time
	UpdatedAt  time.Time
	// Players is who a Director chose for the deck. The repository fills it
	// on every deck read, so no caller can decide who sees a deck without it.
	Players []Player
}

// Player is one player a Director chose for a deck.
type Player struct {
	UserID string
	// DrawsLeft is how many draws the player has left; nil is no limit.
	DrawsLeft *int
	// GrantedAt is when the Director chose the player; a grant older than the
	// member's joined_at belongs to an earlier stay and counts for nothing.
	GrantedAt time.Time
}

// Card is a card definition.
type Card struct {
	ID           string
	DeckID       string
	CampaignID   string
	Name         string
	KindKey      string
	Words        string
	DirectorNote string
	Picture      string
	Corner       string
	// Rare is part of the card's face: it is sent only with a face.
	Rare   bool
	Copies int
	// Fate is "" for the deck's default.
	Fate string
	// When holds a condition; it is always empty until conditions exist.
	When      string
	WhenOpen  bool
	AskFirst  bool
	SortOrder int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Copy is one physical card.
type Copy struct {
	ID             string
	DeckID         string
	CampaignID     string
	CardID         string
	Zone           string
	HolderEntityID string
	FaceUp         bool
	// Open is a face-up card in a hand that its holder has shown to the table.
	// It implies FaceUp and is cleared whenever the copy leaves the hand.
	Open     bool
	AskedAt  *time.Time
	DealtBy  string
	DealtAt  *time.Time
	TurnedAt *time.Time
	MovedAt  time.Time
}

// LogLine is one move, for Directors.
type LogLine struct {
	ID             int64     `json:"id"`
	DeckID         string    `json:"deckId"`
	CardID         string    `json:"cardId,omitempty"`
	CardName       string    `json:"cardName"`
	CopyID         string    `json:"copyId,omitempty"`
	Action         string    `json:"action"`
	ActorUserID    string    `json:"actorUserId"`
	HolderEntityID string    `json:"holderEntityId,omitempty"`
	Seen           bool      `json:"seen"`
	CreatedAt      time.Time `json:"createdAt"`
}

// Hero is a page a hero deck can belong to and a player can own.
type Hero struct {
	ID   string
	Name string
	// OwnerUserID is the claimed owner, "" when unclaimed.
	OwnerUserID string
}

// ---- Inputs -------------------------------------------------------------

// CreateDeckInput makes a deck. Settings and Look are optional; whatever is
// left out takes the kind's default.
type CreateDeckInput struct {
	Name           string         `json:"name"`
	Kind           string         `json:"kind"`
	HolderEntityID string         `json:"holderEntityId"`
	Settings       *SettingsInput `json:"settings"`
	Look           *LookInput     `json:"look"`
}

// UpdateDeckInput is a partial update: an absent key keeps the stored value,
// an explicit null clears the hero page, a value replaces. Version is
// required and is the version the editor loaded.
type UpdateDeckInput struct {
	Version        patch.Field[int]           `json:"version"`
	Name           patch.Field[string]        `json:"name"`
	Kind           patch.Field[string]        `json:"kind"`
	HolderEntityID patch.Field[string]        `json:"holderEntityId"`
	Settings       patch.Field[SettingsInput] `json:"settings"`
	Look           patch.Field[LookInput]     `json:"look"`
	// NextCardID and LastCardID are Director-only: absent keeps, null or ""
	// clears, a card id of this deck sets.
	NextCardID patch.Field[string] `json:"nextCardId"`
	LastCardID patch.Field[string] `json:"lastCardId"`
}

// PlayerInput chooses one player and how many draws they have.
type PlayerInput struct {
	UserID string `json:"userId"`
	// DrawsLeft nil is no limit.
	DrawsLeft *int `json:"drawsLeft"`
}

// SetPlayersInput replaces a deck's chosen players.
type SetPlayersInput struct {
	Version int           `json:"version"`
	Players []PlayerInput `json:"players"`
}

// CreateCardInput makes a card; Copies defaults to 1.
type CreateCardInput struct {
	Name         string `json:"name"`
	KindKey      string `json:"kindKey"`
	Words        string `json:"words"`
	DirectorNote string `json:"directorNote"`
	Picture      string `json:"picture"`
	Corner       string `json:"corner"`
	Rare         bool   `json:"rare"`
	Copies       *int   `json:"copies"`
	Fate         string `json:"fate"`
	When         string `json:"when"`
	WhenOpen     bool   `json:"whenOpen"`
	AskFirst     bool   `json:"askFirst"`
}

// UpdateCardInput is a partial update of a card.
type UpdateCardInput struct {
	Name         patch.Field[string] `json:"name"`
	KindKey      patch.Field[string] `json:"kindKey"`
	Words        patch.Field[string] `json:"words"`
	DirectorNote patch.Field[string] `json:"directorNote"`
	Picture      patch.Field[string] `json:"picture"`
	Corner       patch.Field[string] `json:"corner"`
	Rare         patch.Field[bool]   `json:"rare"`
	Copies       patch.Field[int]    `json:"copies"`
	Fate         patch.Field[string] `json:"fate"`
	When         patch.Field[string] `json:"when"`
	WhenOpen     patch.Field[bool]   `json:"whenOpen"`
	AskFirst     patch.Field[bool]   `json:"askFirst"`
}

// DrawInput names the hero a player draws into; a Director's draw goes to the
// shown spot and names none. Count is how many cards a Director takes, 1 to 5,
// and 0 means one; a player's draw is always one card.
type DrawInput struct {
	HeroID string `json:"heroId"`
	Count  int    `json:"count"`
}

// ActInput carries what a verb needs: where a deal goes, or the deck a card is
// kept in.
type ActInput struct {
	// To is "hand" or "table" for deal. A hand deal on a party deck with no
	// HeroID lands in the party's own hand.
	To     string `json:"to"`
	HeroID string `json:"heroId"`
	// TargetDeckID is the deck `keep` copies the card into.
	TargetDeckID string `json:"targetDeckId"`
}

// DealCardInput deals a named card's pile copy.
type DealCardInput struct {
	CardID string `json:"cardId"`
	To     string `json:"to"`
	HeroID string `json:"heroId"`
}

// ---- Director responses ------------------------------------------------

// DeckView is what Get and every move return. Only DirectorDeck and
// PlayerDeck implement it, so a handler cannot send anything else.
type DeckView interface{ deckView() }

// DeckListView is what List returns: DirectorList or PlayerList.
type DeckListView interface{ deckListView() }

// ZoneCounts is how many copies a deck has in each zone.
type ZoneCounts struct {
	Pile  int `json:"pile"`
	Shown int `json:"shown"`
	Hand  int `json:"hand"`
	Play  int `json:"play"`
	Used  int `json:"used"`
	Gone  int `json:"gone"`
}

// DirectorPlayer is a chosen player as a Director sees them.
type DirectorPlayer struct {
	UserID    string `json:"userId"`
	DrawsLeft *int   `json:"drawsLeft"`
}

// DirectorCard is a card with everything a Director wrote.
type DirectorCard struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	KindKey      string `json:"kindKey"`
	Words        string `json:"words"`
	DirectorNote string `json:"directorNote"`
	Picture      string `json:"picture"`
	Corner       string `json:"corner"`
	Rare         bool   `json:"rare"`
	Copies       int    `json:"copies"`
	Fate         string `json:"fate"`
	When         string `json:"when"`
	WhenOpen     bool   `json:"whenOpen"`
	AskFirst     bool   `json:"askFirst"`
	SortOrder    int    `json:"sortOrder"`
}

// DirectorCopy is one copy wherever it is.
type DirectorCopy struct {
	ID             string     `json:"id"`
	CardID         string     `json:"cardId"`
	Zone           string     `json:"zone"`
	HolderEntityID string     `json:"holderEntityId,omitempty"`
	FaceUp         bool       `json:"faceUp"`
	Open           bool       `json:"open"`
	AskedAt        *time.Time `json:"askedAt,omitempty"`
	DealtBy        string     `json:"dealtBy,omitempty"`
	DealtAt        *time.Time `json:"dealtAt,omitempty"`
	TurnedAt       *time.Time `json:"turnedAt,omitempty"`
	MovedAt        time.Time  `json:"movedAt"`
}

// DirectorDeck is a whole deck for a Director. Cards, Copies and Log are
// filled for one deck (Get and moves) and left out of the list.
type DirectorDeck struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Kind           string           `json:"kind"`
	HolderEntityID string           `json:"holderEntityId,omitempty"`
	NextCardID     string           `json:"nextCardId"`
	LastCardID     string           `json:"lastCardId"`
	Settings       Settings         `json:"settings"`
	Look           Look             `json:"look"`
	Version        int              `json:"version"`
	Players        []DirectorPlayer `json:"players"`
	Counts         ZoneCounts       `json:"counts"`
	Cards          []DirectorCard   `json:"cards,omitempty"`
	Copies         []DirectorCopy   `json:"copies,omitempty"`
	Log            []LogLine        `json:"log,omitempty"`
}

func (DirectorDeck) deckView() {}

// DirectorList is the Director's list of decks.
type DirectorList struct {
	Decks []DirectorDeck `json:"decks"`
}

func (DirectorList) deckListView() {}

// ---- Player responses ---------------------------------------------------
//
// These are separate types, never a Director type with fields blanked: a
// field a player must not see is not on them at all.

// Face is what a player reads on a card that is face up: never the Director's
// note or a condition. Picture is the card's media file id; the media server
// decides who may fetch it (ADR-078), and a face-down copy carries none.
type Face struct {
	CardID  string `json:"cardId"`
	Name    string `json:"name"`
	KindKey string `json:"kindKey"`
	Words   string `json:"words"`
	Picture string `json:"picture"`
	Corner  string `json:"corner"`
	Rare    bool   `json:"rare"`
}

// PlayerCopy is a copy a player may know about. Face is present only for a
// copy that is face up; a face-down copy is its id, its deck and that it is
// face down.
type PlayerCopy struct {
	ID     string `json:"id"`
	DeckID string `json:"deckId"`
	FaceUp bool   `json:"faceUp"`
	// Waiting is true while a play of this card waits for a Director.
	Waiting bool `json:"waiting"`
	// Open is true on a face-up card its holder has shown to the table. It is
	// never set on a face-down copy, so a face-down entry is always the same
	// four keys.
	Open bool `json:"open,omitempty"`
	// AskFirst is true on a face-up card whose play waits for a Director's
	// answer, so the holder's button reads "Ask to play". It is never set on a
	// face-down copy.
	AskFirst bool  `json:"askFirst,omitempty"`
	Face     *Face `json:"face,omitempty"`
}

// PlayerHand is one of the viewer's own heroes and what they hold of a deck.
type PlayerHand struct {
	HeroID string       `json:"heroId"`
	Hand   []PlayerCopy `json:"hand"`
	Play   []PlayerCopy `json:"play"`
}

// KindCount is how many cards of a kind are in the pile.
type KindCount struct {
	KindKey string `json:"kindKey"`
	Count   int    `json:"count"`
}

// OtherHolds is what another hero holds, by what the deck lets others see.
type OtherHolds struct {
	HeroID string `json:"heroId"`
	Count  int    `json:"count"`
	// Faces are the face-up cards, sent only when others may see faces.
	Faces []Face `json:"faces,omitempty"`
}

// PlayerDeck is a deck as a listed player sees it.
type PlayerDeck struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Look      Look   `json:"look"`
	CanDraw   bool   `json:"canDraw"`
	DrawsLeft *int   `json:"drawsLeft"`
	// DrawsOpen is true for a player the deck lets draw for themselves, even
	// with no draws left, so the screen can say so.
	DrawsOpen bool         `json:"drawsOpen"`
	Hands     []PlayerHand `json:"hands"`
	// PartyHand is the party deck's own hand, held by no hero and seen by every
	// member of the campaign. Face-down copies are face-down entries.
	PartyHand []PlayerCopy `json:"partyHand,omitempty"`
	// Pile is the pile's count, present when the deck lets players know it.
	Pile *int `json:"pile,omitempty"`
	// Kinds is the pile's count per kind, present for "kinds" and "faces".
	Kinds []KindCount `json:"kinds,omitempty"`
	// Cards is every card's face, present only for "faces".
	Cards []Face `json:"cards,omitempty"`
	// Others is what other heroes hold, by the deck's othersSee.
	Others []OtherHolds `json:"others,omitempty"`
	// Used is the used pile of a party deck, visible to every member.
	Used []PlayerCopy `json:"used,omitempty"`
}

func (PlayerDeck) deckView() {}

// PlayerList is a player's list of decks.
type PlayerList struct {
	Decks []PlayerDeck `json:"decks"`
}

func (PlayerList) deckListView() {}

// ActionResult is what a move returns: the copy it touched and the deck as
// the same viewer now sees it.
type ActionResult struct {
	CopyID string `json:"copyId,omitempty"`
	// CopyIDs lists the copies a draw took, a list even when it holds one. A
	// player's draw has the same shape.
	CopyIDs []string `json:"copyIds,omitempty"`
	Deck    DeckView `json:"deck"`
}

// TableCard is a card lying on the table, for every member.
type TableCard struct {
	CopyID string `json:"copyId"`
	Face   Face   `json:"face"`
}

// TableDeck is one deck's cards on the table.
type TableDeck struct {
	DeckID string      `json:"deckId"`
	Name   string      `json:"name"`
	Kind   string      `json:"kind"`
	Look   Look        `json:"look"`
	Cards  []TableCard `json:"cards"`
}

// TableView is every card lying in front of the whole table.
type TableView struct {
	Decks []TableDeck `json:"decks"`
}
