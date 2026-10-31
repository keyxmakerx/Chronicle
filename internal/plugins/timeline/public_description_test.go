package timeline

import (
	"strings"
	"testing"
)

// A timeline's or standalone event's description is served as stored to
// everyone who sees it, so GM-only parts sent through the API or an import
// never reach storage.
func TestTimelineDescriptionsDropGMOnlyParts(t *testing.T) {
	html := `<p>The siege. <span data-secret="true">The gate was opened from inside.</span></p>`
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"The siege. "},{"type":"text","text":"opened from inside","marks":[{"type":"secret"}]}]}]}`
	if got := publicHTML(html); strings.Contains(got, "inside") || !strings.Contains(got, "The siege.") {
		t.Errorf("HTML kept the GM-only part: %q", got)
	}
	if got := publicDescription(&doc); strings.Contains(*got, "inside") || !strings.Contains(*got, "The siege.") {
		t.Errorf("JSON kept the GM-only part: %q", *got)
	}
}
