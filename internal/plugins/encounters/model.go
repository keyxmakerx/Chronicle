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
	Version   *int       `json:"version"`
}

// doc is the JSON stored in the data column.
type doc struct {
	Heroes    []string   `json:"heroes"`
	Creatures []Creature `json:"creatures"`
}
