// system_id_xss_test.go — regression pin for C-SEC-XSS-JSATTR-SWEEP-R1 sink 3:
// a campaign's SystemID flowed verbatim into the Game System selector's Alpine
// `x-data` expression (`_savedSystemId: '%s'`). SystemID accepts an
// owner-supplied `custom:<url>` value with no server-side validation, so the
// remainder is free text that could break out of the JS string literal. This is
// owner-only to set and owner-only to view, so it is self-XSS (LOW) — but the
// sink is hardened anyway with jsEsc (the dispatch deliberately did NOT add new
// `custom:`-remainder validation machinery for a self-XSS).
//
// See parent_selector_xss_test.go for why the rendered discriminator is the
// backslash: fixed renders `custom:\&#39;` (escaped quote), vulnerable renders
// `custom:&#39;` immediately followed by the breakout.

package campaigns

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestSettingsGeneralTab_EscapesHostileSystemID pins that a hostile SystemID
// cannot break out of the _savedSystemId JS string literal.
func TestSettingsGeneralTab_EscapesHostileSystemID(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{
		ID:       "camp-1",
		Name:     "Test",
		Settings: `{"system_id":"custom:');alert(1)//"}`,
	}}

	var buf bytes.Buffer
	if err := settingsGeneralTab(cc, "csrf", "[]").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	html := buf.String()

	// Unescaped breakout — the custom: prefix immediately followed by the quote
	// entity and payload — must NOT appear.
	if strings.Contains(html, "custom:&#39;);alert(1)") {
		t.Errorf("settings general tab let a hostile SystemID break out of _savedSystemId; jsEsc missing at the sink\nrendered: %s", html)
	}
	// Escaped form — a backslash before the injected quote entity — MUST appear.
	if !strings.Contains(html, `custom:\&#39;);alert(1)`) {
		t.Errorf("expected the injected quote to be backslash-escaped (jsEsc) in _savedSystemId; not found\nrendered: %s", html)
	}
}

// TestSettingsGeneralTab_LegitSystemIDUnchanged pins that an ordinary system id
// still renders intact, so the fix does not regress legitimate values.
func TestSettingsGeneralTab_LegitSystemIDUnchanged(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{
		ID:       "camp-1",
		Name:     "Test",
		Settings: `{"system_id":"dnd5e"}`,
	}}

	var buf bytes.Buffer
	if err := settingsGeneralTab(cc, "csrf", "[]").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !strings.Contains(buf.String(), "_savedSystemId: &#39;dnd5e&#39;") {
		t.Errorf("legit system id was not rendered intact\nrendered: %s", buf.String())
	}
}
