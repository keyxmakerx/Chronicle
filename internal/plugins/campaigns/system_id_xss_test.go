// system_id_xss_test.go pins that a campaign's SystemID reaches the game
// system picker only as an HTML-escaped data-* attribute value. SystemID
// accepts an owner-supplied `custom:<url>` with no server-side validation, so
// it must never become part of the Alpine x-data script text.

package campaigns

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func renderGameSystemCard(t *testing.T, settings string) string {
	t.Helper()
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1", Name: "Test", Settings: settings}}
	var buf bytes.Buffer
	if err := gameSystemCard(cc, "[]", currentSystemID(cc.Campaign.ParseSettings().SystemID, nil)).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	return buf.String()
}

func TestGameSystemCard_HostileSystemIDStaysInDataAttribute(t *testing.T) {
	html := renderGameSystemCard(t, `{"system_id":"custom:');alert(1)//"}`)

	// The payload may only appear HTML-escaped inside the data attribute.
	if !strings.Contains(html, `data-saved-system="custom:&#39;);alert(1)//"`) {
		t.Errorf("expected the system id as an escaped data attribute\nrendered: %s", html)
	}
	if strings.Count(html, "alert(1)") != 1 {
		t.Errorf("hostile system id appears outside its data attribute\nrendered: %s", html)
	}
}

func TestGameSystemCard_LegitSystemIDUnchanged(t *testing.T) {
	html := renderGameSystemCard(t, `{"system_id":"dnd5e"}`)
	if !strings.Contains(html, `data-saved-system="dnd5e"`) {
		t.Errorf("legit system id was not rendered intact\nrendered: %s", html)
	}
}

// TestSettingsGeneralTab_NoLongerHoldsPicker pins that the picker lives in one
// place: two live controls for one setting would drift.
func TestSettingsGeneralTab_NoLongerHoldsPicker(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1", Name: "Test"}}
	var buf bytes.Buffer
	if err := settingsGeneralTab(cc, "csrf").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	html := buf.String()
	if strings.Contains(html, "data-game-system-card") {
		t.Error("General tab still renders the game system picker")
	}
	if !strings.Contains(html, "/campaigns/camp-1/extensions") {
		t.Error("General tab should link to the Game & features page")
	}
}

// The save posts to the campaign read in init(). Reading $el.dataset inside
// the @change handler gets the select's dataset, which has no campaign id,
// and the save went to /campaigns/undefined/system.
func TestGameSystemCard_SaveUsesCampaignReadInInit(t *testing.T) {
	html := renderGameSystemCard(t, `{"system_id":"dnd5e"}`)
	if !strings.Contains(html, `data-campaign-id="camp-1"`) {
		t.Errorf("campaign id attribute missing\nrendered: %s", html)
	}
	if got := strings.Count(html, "$el.dataset"); got != 1 {
		t.Errorf("$el.dataset read %d times, want once (in init)\nrendered: %s", got, html)
	}
	if !strings.Contains(html, "this.campaignId + ") {
		t.Errorf("save does not build its URL from the stored campaign id\nrendered: %s", html)
	}
}
