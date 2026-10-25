package sanitize

import (
	"strings"
	"testing"
)

// TestHTML_KeepsSceneMentionIDs: a Foundry scene mention survives sanitizing
// with its scene id, and an id that is not plain letters and digits is
// dropped, leaving a mention that goes by name.
func TestHTML_KeepsSceneMentionIDs(t *testing.T) {
	got := HTML(`<p><span data-type="foundry-scene" class="scene-mention" data-scene-id="Ab12Cd34Ef56Gh78">Sunken Crypt</span></p>`)
	for _, want := range []string{`data-type="foundry-scene"`, `data-scene-id="Ab12Cd34Ef56Gh78"`, "Sunken Crypt"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}

	got = HTML(`<span data-type="foundry-scene" data-scene-id="&quot; onclick=&quot;x">a</span>` +
		`<span data-type="foundry-scene" data-scene-id="../x">b</span><a data-scene-id="Ab12">c</a>`)
	if strings.Contains(got, "data-scene-id") || strings.Contains(got, "onclick") {
		t.Errorf("a malformed or misplaced scene id must be dropped: %s", got)
	}
}
