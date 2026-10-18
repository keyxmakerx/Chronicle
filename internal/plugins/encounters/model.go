package encounters

import "time"

// BlockType is the layout block that shows a page's encounters.
const BlockType = "page_encounters"

// How a creature starts the fight. Foundry places a "hidden" creature with its
// token hidden and leaves a "later" one off the scene until the DM places it;
// nothing is ever revealed without the DM pressing Show.
const (
	StartSeen   = "seen"
	StartHidden = "hidden"
	StartLater  = "later"
)

// Viewer is who is asking, taken from the campaign context in the handler.
type Viewer struct {
	UserID string
	// VisibilityRole is the role used for what a viewer may see; a member the
	// owner gave DM access counts as an owner there.
	VisibilityRole int
}

// Creature is one row of an encounter: Count copies of one creature.
type Creature struct {
	ID string `json:"id"`
	// EntityID is the creature's page, which Foundry maps to its actor.
	// Empty for a creature typed by name that has no page yet.
	EntityID string `json:"entityId"`
	// Name is kept with the row so the encounter still reads correctly when
	// the page is renamed or removed.
	Name  string `json:"name"`
	Count int    `json:"count"`
	Group string `json:"group"`
	Start string `json:"start"`
}

// How a form change is shown to players. "reveal" tells them; "quiet" swaps
// the token without announcing it.
const (
	HowReveal = "reveal"
	HowQuiet  = "quiet"
)

// When a stage is due. Only the DM presses Start; the kind just tells the DM
// Screen when to say a stage is waiting.
const (
	WhenManual  = "manual"
	WhenStamina = "stamina"
	WhenRound   = "round"
)

// Stage says when something changes mid-fight: creatures take another form
// and/or "later" rows arrive. It never fires on its own.
type Stage struct {
	ID     string      `json:"id"`
	When   StageWhen   `json:"when"`
	Forms  []StageForm `json:"forms"`
	Arrive []string    `json:"arrive"`
	How    string      `json:"how"`
	// Note is the DM's reminder, plain text.
	Note string `json:"note"`
}

// StageWhen is what makes a stage due. Row, Percent and Round are read only
// for the kind that uses them.
type StageWhen struct {
	Kind string `json:"kind"`
	// Row is the creature row whose Stamina is watched (stamina).
	Row string `json:"row"`
	// Percent is due at or below this share of Stamina, 1 to 99 (stamina).
	Percent int `json:"percent"`
	// Round is due at the start of this round, 2 to 99 (round).
	Round int `json:"round"`
}

// StageForm is one creature changing form.
type StageForm struct {
	Row string `json:"row"`
	// Form is the game system's form id, or empty for the creature's first.
	Form string `json:"form"`
	// FormName is kept so the stage reads right if the system's forms change.
	FormName string `json:"formName"`
}

// Hero is one hero page the encounter was built for.
type Hero struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Page names the page an encounter is pinned to.
type Page struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Encounter is one planned fight as the API returns it.
type Encounter struct {
	ID        string     `json:"id"`
	Page      Page       `json:"page"`
	Name      string     `json:"name"`
	Ready     bool       `json:"ready"`
	Heroes    []Hero     `json:"heroes"`
	Creatures []Creature `json:"creatures"`
	Stages    []Stage    `json:"stages"`
	Version   int        `json:"version"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// Summary is one line of the Foundry list.
type Summary struct {
	ID            string    `json:"id"`
	Page          Page      `json:"page"`
	Name          string    `json:"name"`
	Ready         bool      `json:"ready"`
	CreatureCount int       `json:"creatureCount"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// PageList is what the page's widget receives.
type PageList struct {
	Encounters []Encounter `json:"encounters"`
	CanEdit    bool        `json:"canEdit"`
}

// SaveInput is a create or update body. The builder always sends the whole
// encounter. Version is a pointer so a missing one is refused on update
// rather than read as "no row yet".
type SaveInput struct {
	Name      string     `json:"name"`
	Ready     bool       `json:"ready"`
	Heroes    []string   `json:"heroes"`
	Creatures []Creature `json:"creatures"`
	Stages    []Stage    `json:"stages"`
	Version   *int       `json:"version"`
}

// doc is the JSON stored in the data column.
// Documents saved before stages existed have no "stages" member and read as
// having none.
type doc struct {
	Heroes    []string   `json:"heroes"`
	Creatures []Creature `json:"creatures"`
	Stages    []Stage    `json:"stages"`
}
