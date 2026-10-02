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
	if err := gameSystemCard(cc, "[]").Render(context.Background(), &buf); err != nil {
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
		t.Error("General tab should link to the Apps & game system page")
	}
}
