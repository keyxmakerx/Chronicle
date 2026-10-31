package rolls

import "time"

// Who may see a roll, the same four modes Foundry has.
const (
	// ModePublic is seen by everyone at the table.
	ModePublic = "public"
	// ModeGM is seen by the Directors and the roller.
	ModeGM = "gm"
	// ModeBlind is seen by the Directors; the roller learns only that it was made.
	ModeBlind = "blind"
	// ModeSelf is seen by the roller alone.
	ModeSelf = "self"
)

// Which app decided a roll's numbers.
const (
	OriginChronicle = "chronicle"
	OriginFoundry   = "foundry"
)

// What made a roll inside its app.
const (
	SourceTable    = "table"
	SourceCrafting = "crafting"
	SourceFoundry  = "foundry"
)

// The table's dice looks. Each person picks their own; a new player starts on
// the table's.
const (
	LookQuick = "quick"
	LookRune  = "rune"
	LookNone  = "none"
)

// Bounds on one roll, so the log stays small and a hostile client can at worst
// fill its own campaign's log with short rows.
const (
	MaxDice       = 100
	MaxSides      = 1000
	MaxModifier   = 9999
	MaxTotal      = 1000000
	FormulaMax    = 120
	LabelMax      = 160
	NameMax       = 100
	OutcomeMax    = 160
	WhyMax        = 300
	FromMax       = 160
	OriginIDMax   = 80
	DefaultPage   = 50
	MaxPage       = 200
	tableDiceMax  = 50
	tableTermsMax = 20
)

// Die is one die as it fell.
type Die struct {
	Sides int `json:"sides"`
	Value int `json:"value"`
}

// Roll is one roll in the log.
type Roll struct {
	ID         string    `json:"id"`
	CampaignID string    `json:"-"`
	Origin     string    `json:"origin"`
	OriginID   string    `json:"originId,omitempty"`
	Source     string    `json:"source"`
	RollerID   string    `json:"rollerId,omitempty"`
	RollerName string    `json:"rollerName"`
	Label      string    `json:"label"`
	Formula    string    `json:"formula"`
	Dice       []Die     `json:"dice,omitempty"`
	Modifier   int       `json:"modifier"`
	Total      int       `json:"total"`
	Outcome    string    `json:"outcome,omitempty"`
	Why        string    `json:"why,omitempty"`
	From       string    `json:"from,omitempty"`
	Mode       string    `json:"mode"`
	CreatedAt  time.Time `json:"createdAt"`
}

// View is a roll as one viewer is told it. A blind roll reaches its roller
// with Hidden set and no dice, modifier, total or outcome.
type View struct {
	ID         string    `json:"id"`
	Origin     string    `json:"origin"`
	OriginID   string    `json:"originId,omitempty"`
	Source     string    `json:"source"`
	RollerID   string    `json:"rollerId,omitempty"`
	RollerName string    `json:"rollerName"`
	Label      string    `json:"label"`
	Formula    string    `json:"formula"`
	Dice       []Die     `json:"dice,omitempty"`
	Modifier   *int      `json:"modifier,omitempty"`
	Total      *int      `json:"total,omitempty"`
	Outcome    string    `json:"outcome,omitempty"`
	Why        string    `json:"why,omitempty"`
	From       string    `json:"from,omitempty"`
	Mode       string    `json:"mode"`
	Hidden     bool      `json:"hidden,omitempty"`
	Mine       bool      `json:"mine,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Viewer is who is asking. Director is true for the owner and members with
// DM access (CampaignContext.VisibilityRole at Owner).
type Viewer struct {
	UserID   string
	Name     string
	Director bool
}

// TableInput is a roll asked for at the Table tab.
type TableInput struct {
	Formula string `json:"formula"`
	Label   string `json:"label"`
	Mode    string `json:"mode"`
}

// ReportInput is a roll another app decided and reports (Foundry). RollerID
// is resolved by the caller from the app's own user, or empty.
type ReportInput struct {
	Origin     string
	OriginID   string
	RollerID   string
	RollerName string
	Label      string
	Formula    string
	Dice       []Die
	Total      int
	Mode       string
}

// LogInput is a roll another plugin made on the server and adds to the log.
type LogInput struct {
	Source     string
	RollerID   string
	RollerName string
	Label      string
	Formula    string
	Dice       []Die
	Modifier   int
	Total      int
	Outcome    string
	Why        string
	From       string
	Mode       string
}

// Page is one page of the log, oldest first, with whether older rolls exist.
type Page struct {
	Rolls []View `json:"rolls"`
	More  bool   `json:"more"`
}

// Settings is the table's dice look.
type Settings struct {
	Look string `json:"look"`
}
