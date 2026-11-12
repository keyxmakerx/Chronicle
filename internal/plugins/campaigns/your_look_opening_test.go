package campaigns

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// With nothing open to the viewer the page says who decides and offers no
// control; with parts open it opens on one line above them.
func TestYourLookPage_Opening(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "c1", Name: "Ashen"}}
	render := func(parts []yourLookPart) string {
		var buf bytes.Buffer
		if err := YourLookPage(cc, parts).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	none := render(nil)
	if !strings.Contains(none, `data-app-opening="empty"`) || !strings.Contains(none, "None are open to you yet") {
		t.Errorf("an empty Your look should open large with who decides:\n%s", none)
	}
	sheet, _ := LookPartByKey(LookPartSheet)
	some := render([]yourLookPart{{Part: sheet, URL: "/x"}})
	if !strings.Contains(some, `data-app-opening="full"`) || !strings.Contains(some, `data-look-part="sheet"`) {
		t.Errorf("Your look with parts should open on one line above them:\n%s", some)
	}
}
