package pageblocks

import "encoding/json"

// Block kinds. They equal the layout block types so the widget can ask for
// its content by the type it was placed as.
const (
	KindReadAloud = "read_aloud"
	KindTable     = "page_table"
	KindSteps     = "page_steps"
	KindPageMap   = "page_map"
	KindChecklist = "page_checklist"
)

// Viewer is who is asking, taken from the campaign context in the handler.
type Viewer struct {
	UserID string
	// MemberRole is the real membership role (0 guest .. 3 owner).
	MemberRole int
	// VisibilityRole is the role used for what a viewer may see; a member the
	// owner gave DM access counts as an owner there. It decides who sees a
	// DM-only column.
	VisibilityRole int
}

// ReadAloud is the boxed text a DM reads to the table.
type ReadAloud struct {
	Text string `json:"text"`
}

// Column is one table column. DMOnly columns are removed, with their cells,
// for any viewer below Owner visibility.
type Column struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	DMOnly bool   `json:"dmOnly"`
}

// Row is one table row; Cells is keyed by column id.
type Row struct {
	ID    string            `json:"id"`
	Cells map[string]string `json:"cells"`
}

// Table is the Table block's document.
type Table struct {
	Columns []Column `json:"columns"`
	Rows    []Row    `json:"rows"`
}

// Step is one beat of the Steps block.
type Step struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
	Done  bool   `json:"done"`
}

// Steps is the Steps block's document.
type Steps struct {
	Steps []Step `json:"steps"`
}

// Holder sides of a checklist item. An item nobody holds yet has "".
const (
	SideA = "a"
	SideB = "b"
)

// ChecklistItem is one named thing in a Checklist, held by side a, side b or
// nobody. EntityID optionally links it to a page; EntityName and EntityURL
// are added on read for the links the viewer may open, and ignored on save.
// DM items are removed for a viewer below Owner visibility.
type ChecklistItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	EntityID   string `json:"entityId,omitempty"`
	EntityName string `json:"entityName,omitempty"`
	EntityURL  string `json:"entityUrl,omitempty"`
	Holder     string `json:"holder"`
	DM         bool   `json:"dm"`
}

// Threshold is a count of items one side may reach, with a note for the DM
// when it does. Thresholds are the DM's alone: they never reach a viewer
// below Owner visibility.
type Threshold struct {
	ID    string `json:"id"`
	Side  string `json:"side"`
	Count int    `json:"count"`
	Note  string `json:"note"`
}

// Checklist is the Checklist block's document: named items, each held by one
// of two sides the owner names. DMOnly hides the whole block from players.
type Checklist struct {
	Title      string          `json:"title"`
	SideA      string          `json:"sideA"`
	SideB      string          `json:"sideB"`
	DMOnly     bool            `json:"dmOnly"`
	Items      []ChecklistItem `json:"items"`
	Thresholds []Threshold     `json:"thresholds"`
}

// View is what the widget receives: the document already filtered for the
// viewer, its version for the next save, and whether the viewer may save.
type View struct {
	Kind    string `json:"kind"`
	Data    any    `json:"data"`
	Version int    `json:"version"`
	CanEdit bool   `json:"canEdit"`
}

// SaveInput is a PUT body. Version is a pointer so a missing one is refused
// rather than read as "no row yet".
type SaveInput struct {
	Kind    string          `json:"kind"`
	Data    json.RawMessage `json:"data"`
	Version *int            `json:"version"`
}
