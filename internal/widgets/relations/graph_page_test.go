package relations

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The graph page introduces itself in the reader's words: pages, not
// entities or nodes.
func TestGraphPage_IntroducesItselfInPlainWords(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", Name: "The Sunken Crown"}}
	var b strings.Builder
	if err := GraphPage(cc, nil).Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := b.String()
	if want := "How the pages of this campaign connect. Click a page to open it."; !strings.Contains(out, want) {
		t.Errorf("the page does not say %q", want)
	}
	for _, old := range []string{"entity relationships", "Click a node", "that entity"} {
		if strings.Contains(out, old) {
			t.Errorf("the page still says %q", old)
		}
	}
}
