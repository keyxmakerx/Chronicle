package transformations

import (
	"encoding/json"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// How one change treats the field it names. The sheet that draws the hero
// decides what a value means; these say only how it combines with the
// field underneath.
const (
	// OpSet replaces the field's value.
	OpSet = "set"
	// OpAdd adds a number to a numeric field.
	OpAdd = "add"
	// OpAppend adds an entry to a field that holds a list.
	OpAppend = "append"
	// OpRemove hides the list entry with this name.
	OpRemove = "remove"
)

// Viewer is who is asking, taken from the campaign context in the handler.
type Viewer struct {
	UserID string
	// VisibilityRole is the role used for what a viewer may see; a member the
	// owner gave DM access counts as an owner there.
	VisibilityRole int
}

// IsDM is Owner visibility, which an owner and a member with DM access both
// have.
func (v Viewer) IsDM() bool { return v.VisibilityRole >= permissions.RoleOwner }

// Change is one field a layer changes.
type Change struct {
	// Field is the sheet field's key, as the game system names it.
	Field string `json:"field"`
	Op    string `json:"op"`
	// Value is a string, number or JSON object; OpAdd takes a number and
	// OpRemove a name.
	Value json.RawMessage `json:"value"`
	// Label is how the sheet names the change when the field alone says too
	// little, such as the trait an appended entry is.
	Label string `json:"label,omitempty"`
}

// Layer is one transformation on a hero, as stored and sent.
type Layer struct {
	ID       string `json:"id"`
	EntityID string `json:"entityId"`
	Name     string `json:"name"`
	// Source is the preset it came from, such as "drawsteel:revenant", so
	// the sheet can link the rule; empty for a layer made by hand.
	Source    string    `json:"source"`
	Note      string    `json:"note"`
	Changes   []Change  `json:"changes"`
	CreatedAt time.Time `json:"createdAt"`
}

// PageLayers is what a page's sheet reads: its layers, oldest first, and
// whether this viewer may put more on or take them off.
type PageLayers struct {
	Layers  []Layer `json:"layers"`
	CanEdit bool    `json:"canEdit"`
}

// AddInput is a Director putting a layer on. Choices a preset asks for are
// settled before this, so the changes are final.
type AddInput struct {
	Name    string   `json:"name"`
	Source  string   `json:"source"`
	Note    string   `json:"note"`
	Changes []Change `json:"changes"`
}

// doc is the stored JSON beside the name and source columns.
type doc struct {
	Note    string   `json:"note"`
	Changes []Change `json:"changes"`
}
