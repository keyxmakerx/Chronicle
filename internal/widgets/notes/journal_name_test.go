package notes

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The Journal names itself in the list's own bar, as its one h1, and has no
// separate title row above the workspace.
func TestJournal_NameSitsInTheListBar(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", Name: "Ashen"}}
	var buf bytes.Buffer
	if err := JournalFragment(cc, JournalView{}).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Count(out, "<h1") != 1 || !strings.Contains(out, `<h1 class="nl-name cb-title">Journal</h1>`) {
		t.Errorf("want exactly one visible Journal h1:\n%s", out)
	}
	if strings.Index(out, `class="nl-bar"`) > strings.Index(out, `class="nl-name`) {
		t.Errorf("the name should be inside the list bar")
	}
}
