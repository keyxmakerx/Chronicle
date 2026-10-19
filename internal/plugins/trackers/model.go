package trackers

import "time"

// Shapes a kind can take.
const (
	// ShapeCount is a number, optionally bounded, with bands that say what
	// each level means.
	ShapeCount = "count"
	// ShapeSteps is a list of named steps; the value is the index of the
	// current one.
	ShapeSteps = "steps"
	// ShapeClock is a countdown drawn as a circle of segments.
	ShapeClock = "clock"
)

// Who sees a tracker.
const (
	SeeDirectors = "directors"
	SeeEveryone  = "everyone"
)

// Log operations. A step forward or back is an add of +1 or -1.
const (
	OpAdd = "add"
	OpSet = "set"
)

// Where a log line came from. Only "hand" is written today; the others are
// reserved for the rules, item use and AI Import that write the same lines.
const (
	SourceHand = "hand"
)

// Viewer is who is asking, taken from the campaign context in the handler.
type Viewer struct {
	UserID string
	// MemberRole is the real membership role (0 guest .. 3 owner).
	MemberRole int
	// VisibilityRole decides what the viewer may see; a member the owner
	// gave DM access counts as an owner there.
	VisibilityRole int
	// Director is the owner or a co-Director: the only people who define
	// kinds, see hidden trackers and change any tracker.
	Director bool
}

// Step is one named step of a steps kind. Text says what it means.
type Step struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// Band says what a count or clock means from At upward, until the next band.
type Band struct {
	At   int    `json:"at"`
	Text string `json:"text"`
}

// Kind is a campaign's own tracker definition.
type Kind struct {
	ID         string `json:"id"`
	CampaignID string `json:"-"`
	Name       string `json:"name"`
	Shape      string `json:"shape"`
	// Min and Max bound a count (nil is unbounded on that side). A clock's
	// Max is its number of segments. Steps ignores both.
	Min   *int   `json:"min"`
	Max   *int   `json:"max"`
	Steps []Step `json:"steps"`
	Bands []Band `json:"bands"`
	// Visibility is the default for trackers placed from this kind.
	Visibility string `json:"visibility"`
	// EveryTypeID puts this kind on every page of one category, so a
	// per-hero tracker needs no setting up per page. 0 is none.
	EveryTypeID int `json:"everyTypeId"`
	Version     int `json:"version"`
	// Owner names the record another plugin keeps this kind in step with
	// (a Homebrew Workshop recipe's part); "" for a kind made by hand. Such
	// a kind is changed only through its owner.
	Owner string `json:"-"`
	// Managed tells the Director the kind is kept by its owner.
	Managed bool `json:"managed"`
	// InUse counts the trackers placed from this kind; filled for the
	// kinds list only.
	InUse int `json:"inUse"`
}

// Tracker is a kind placed on a page.
type Tracker struct {
	ID         string
	CampaignID string
	EntityID   string
	KindID     string
	Name       string
	Visibility string
	SortOrder  int
}

// Entry is one log line.
type Entry struct {
	ID        int64
	TrackerID string
	Op        string
	Amount    int
	Note      string
	Source    string
	Game      *GameDate
	CreatedBy string
	CreatedAt time.Time
	StruckAt  *time.Time
}

// GameDate is an in-game date from the campaign's default calendar.
type GameDate struct {
	Year  int    `json:"year"`
	Month int    `json:"month"`
	Day   int    `json:"day"`
	Label string `json:"label"`
}

// EntryView is a log line as the widget shows it.
type EntryView struct {
	ID     int64  `json:"id"`
	Op     string `json:"op"`
	Amount int    `json:"amount"`
	Note   string `json:"note"`
	Source string `json:"source"`
	// When is the in-game date's label, or "" when there was no calendar.
	When string `json:"when"`
	// At is the real time, for ordering and a tooltip.
	At     time.Time `json:"at"`
	Struck bool      `json:"struck"`
	// After is the value right after this line, so the log reads as a
	// running balance.
	After int `json:"after"`
}

// TrackerView is one tracker as the viewer may see it, with its value worked
// out from the log.
type TrackerView struct {
	// ID is "" for a tracker that a kind puts on every page of a category
	// and that nobody has changed yet; the first change creates it.
	ID         string `json:"id"`
	KindID     string `json:"kindId"`
	Name       string `json:"name"`
	Shape      string `json:"shape"`
	Visibility string `json:"visibility"`
	Value      int    `json:"value"`
	Min        *int   `json:"min"`
	Max        *int   `json:"max"`
	Steps      []Step `json:"steps"`
	Bands      []Band `json:"bands"`
	// Now is what the current value means: the step's or band's text.
	Now      string      `json:"now"`
	Log      []EntryView `json:"log"`
	LogCount int         `json:"logCount"`
}

// KindChoice is a kind a Director can place on a page.
type KindChoice struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Shape      string `json:"shape"`
	Visibility string `json:"visibility"`
}

// PageView is everything the Trackers block needs for one page.
type PageView struct {
	Trackers []TrackerView `json:"trackers"`
	CanEdit  bool          `json:"canEdit"`
	// Kinds is filled for Directors only, for "Add a tracker".
	Kinds []KindChoice `json:"kinds,omitempty"`
}

// KindInput is a create or update body for a kind. Version is required on
// update and a mismatch is a conflict.
type KindInput struct {
	Name        string `json:"name"`
	Shape       string `json:"shape"`
	Min         *int   `json:"min"`
	Max         *int   `json:"max"`
	Steps       []Step `json:"steps"`
	Bands       []Band `json:"bands"`
	Visibility  string `json:"visibility"`
	EveryTypeID int    `json:"everyTypeId"`
	Version     int    `json:"version"`
}

// PlaceInput puts a kind on a page.
type PlaceInput struct {
	KindID     string `json:"kindId"`
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
}

// TrackerPatch renames a tracker or changes who sees it; nil leaves a field.
type TrackerPatch struct {
	Name       *string `json:"name"`
	Visibility *string `json:"visibility"`
}

// LogInput is one change. TrackerID names a placed tracker; a tracker a kind
// puts on every page and that has no row yet is named by KindID instead.
type LogInput struct {
	TrackerID string `json:"trackerId"`
	KindID    string `json:"kindId"`
	Op        string `json:"op"`
	Amount    int    `json:"amount"`
	Note      string `json:"note"`
}

// TypeRef is a page category, for the "on every page of" picker.
type TypeRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
