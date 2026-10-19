package pagetests

import (
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// BlockType is the layout block that shows a page's tests.
const BlockType = "page_tests"

// Which total, once filled, offers a result. The DM still confirms it; a
// result with no trigger is only ever picked by hand.
const (
	OnNone      = ""
	OnSuccesses = "successes"
	OnFailures  = "failures"
)

// Which side of the tally a step changes.
const (
	SideSuccess = "success"
	SideFailure = "failure"
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

// Result is one way a test can end.
type Result struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// On names the total that offers this result when it fills.
	On string `json:"on"`
}

// Day is a date on the campaign calendar.
type Day struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

// Recorded is the result the table settled on.
type Recorded struct {
	Result string `json:"result"`
	// Label is kept so the record still reads right if the result is renamed.
	Label string `json:"label"`
	// Date is the calendar's date when it was recorded; nil when the
	// campaign has no calendar.
	Date      *Day      `json:"date,omitempty"`
	DateLabel string    `json:"dateLabel,omitempty"`
	At        time.Time `json:"at"`
}

// Page names the page a test is on.
type Page struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Test is one test as the API returns it. A player is sent only ID, Name,
// Kind, Shown and Recorded; every other field is left at its zero value.
type Test struct {
	ID    string `json:"id"`
	Page  Page   `json:"page"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Shown bool   `json:"shown"`
	// Needed is the successes that end the test and Limit the failures that
	// end it; 0 means the test has no such total.
	Needed    int       `json:"needed"`
	Limit     int       `json:"limit"`
	Successes int       `json:"successes"`
	Failures  int       `json:"failures"`
	Results   []Result  `json:"results"`
	Recorded  *Recorded `json:"recorded"`
	// Offer is the result whose total has filled, for the DM to confirm;
	// empty once a result is recorded or while no total is full.
	Offer     string    `json:"offer"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// PageList is what the page's block receives.
type PageList struct {
	Tests   []Test `json:"tests"`
	CanEdit bool   `json:"canEdit"`
}

// Choice is one test as the encounter builder offers it for a version's
// pick: every test of the campaign on a page the DM may see.
type Choice struct {
	ID      string   `json:"id"`
	Page    Page     `json:"page"`
	Name    string   `json:"name"`
	Results []Result `json:"results"`
}

// Outcome is what an encounter needs to know about a test: its name, its
// results and the one recorded, if any.
type Outcome struct {
	Name     string
	Results  []Result
	Recorded string
}

// SaveInput is a create or update body: the test's setup. The tally and the
// recorded result have their own calls, so an edit to the setup never
// overwrites a count made at the table. Version is a pointer so a missing
// one is refused on update.
type SaveInput struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Shown   bool     `json:"shown"`
	Needed  int      `json:"needed"`
	Limit   int      `json:"limit"`
	Results []Result `json:"results"`
	Version *int     `json:"version"`
}

// TallyInput moves one side of the tally by one step, up or down.
type TallyInput struct {
	Side    string `json:"side"`
	Step    int    `json:"step"`
	Version *int   `json:"version"`
}

// RecordInput records a result, or clears the record when Result is empty.
// Reset also sets the tally back to zero, for a test the party tries again.
type RecordInput struct {
	Result  string `json:"result"`
	Reset   bool   `json:"reset"`
	Version *int   `json:"version"`
}

// doc is the JSON stored in the data column.
type doc struct {
	Kind      string    `json:"kind"`
	Needed    int       `json:"needed"`
	Limit     int       `json:"limit"`
	Successes int       `json:"successes"`
	Failures  int       `json:"failures"`
	Results   []Result  `json:"results"`
	Recorded  *Recorded `json:"recorded"`
}
