package ai_workspace

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

// Only an add makes a second page beside an existing one, so only an add
// row says it gets a new name.
func TestRenamesOnConflict(t *testing.T) {
	cases := []struct {
		fm   importer.FrontMatter
		want bool
	}{
		{importer.FrontMatter{Action: importer.ActionCreate}, true},
		{importer.FrontMatter{Action: importer.ActionUpdate}, false},
		{importer.FrontMatter{Action: importer.ActionDelete}, false},
		{importer.FrontMatter{Action: importer.ActionCreate, Era: "Age of Ash"}, false},
	}
	for _, c := range cases {
		if got := renamesOnConflict(importer.ParsedPage{FrontMatter: c.fm}); got != c.want {
			t.Errorf("%+v: got %v, want %v", c.fm, got, c.want)
		}
	}
}

// A block pasted again that would make something already there is refused in
// plain words; any other refusal keeps the server's own wording.
func TestAlreadyHaveSay(t *testing.T) {
	exists := records.Plan{Error: `a table called "Tavern names" already exists; use action: update`}
	cases := []struct {
		name   string
		repeat string
		row    RecordRow
		want   string
	}{
		{"repeat create", "13 November 2026", RecordRow{Label: "Table", Name: "Tavern names", Block: 1, Plan: exists}, "You already have the table Tavern names. Nothing was added."},
		{"explicit create", "13 November 2026", RecordRow{Label: "Table", Name: "Tavern names", Action: records.ActionCreate, Block: 1, Plan: exists}, "You already have the table Tavern names. Nothing was added."},
		{"not a repeat", "", RecordRow{Label: "Table", Name: "Tavern names", Block: 1, Plan: exists}, ""},
		{"an update", "13 November 2026", RecordRow{Label: "Table", Name: "Tavern names", Action: records.ActionUpdate, Block: 1, Plan: exists}, ""},
		{"another refusal", "13 November 2026", RecordRow{Label: "Table", Name: "Tavern names", Block: 1, Plan: records.Plan{Error: "a table needs at least one row"}}, ""},
	}
	for _, c := range cases {
		d := ImportReviewData{Repeats: map[int]string{}}
		if c.repeat != "" {
			d.Repeats[0] = c.repeat
		}
		if got := alreadyHaveSay(d, c.row); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
