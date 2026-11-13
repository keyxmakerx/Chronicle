package armory

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// A bare armory opens large with the sentence for the viewer; a filtered
// empty list keeps the small opening and says nothing matched.
func TestArmoryOpening(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", Name: "Ashen"}, MemberRole: campaigns.RolePlayer}
	render := func(total int, opts ItemListOptions) string {
		var buf bytes.Buffer
		if err := ArmoryGalleryContent(cc, nil, total, opts, nil, nil, nil, nil, "").Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	bare := render(0, ItemListOptions{})
	if !strings.Contains(bare, `data-app-opening="empty"`) || strings.Contains(bare, "No items found") {
		t.Errorf("a bare armory should open large without the filter message:\n%s", bare)
	}
	filtered := render(0, ItemListOptions{Search: "zzz"})
	if !strings.Contains(filtered, `data-app-opening="full"`) || !strings.Contains(filtered, "No items found") {
		t.Errorf("a filtered empty list should keep the small opening and say nothing matched:\n%s", filtered)
	}
}
