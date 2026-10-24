package sanitize

import (
	"strings"
	"testing"
)

// A template's writing prompt is an editing hint: players' copies of a page
// never carry it, and the saved HTML never holds it.
func TestTemplatePrompt_NeverReachesPlayersOrHTML(t *testing.T) {
	in := `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Inhabitants"}]},` +
		`{"type":"paragraph","attrs":{"prompt":"Who lives here?"}},` +
		`{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","attrs":{"prompt":"Landmark or feature 1"}}]}]}]}`
	out := StripSecretsJSON(in)
	if strings.Contains(out, "prompt") || strings.Contains(out, "Who lives here") || strings.Contains(out, "Landmark") {
		t.Errorf("player copy still carries a prompt: %s", out)
	}
	if !strings.Contains(out, `"level":2`) || !strings.Contains(out, "Inhabitants") {
		t.Errorf("player copy lost real content: %s", out)
	}

	html := HTML(`<h2>Inhabitants</h2><p data-prompt="Who lives here?"></p>`)
	if strings.Contains(html, "Who lives here") || strings.Contains(html, "data-prompt") {
		t.Errorf("saved HTML holds the prompt: %s", html)
	}
}
