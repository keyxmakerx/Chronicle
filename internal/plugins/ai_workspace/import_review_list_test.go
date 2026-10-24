package ai_workspace

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
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
