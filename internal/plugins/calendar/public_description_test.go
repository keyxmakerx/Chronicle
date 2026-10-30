package calendar

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// An event's description is served as stored to everyone who sees the event,
// so GM-only parts sent through the API or an import never reach storage.
func TestEventDescriptionsDropGMOnlyParts(t *testing.T) {
	html := `<p>Harvest feast. <span data-secret="true">The wine is poisoned.</span></p>`
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Harvest feast. "},{"type":"text","text":"The wine is poisoned.","marks":[{"type":"secret"}]}]}]}`
	plain := "The wine is poisoned, says the bard."

	if got := sanitizeDescriptionHTML(&html); strings.Contains(*got, "poisoned") || !strings.Contains(*got, "Harvest feast.") {
		t.Errorf("create kept the GM-only part: %q", *got)
	}
	if got := sanitizeDescriptionHTMLField(patch.Of(html)); strings.Contains(*got, "poisoned") {
		t.Errorf("update kept the GM-only part: %q", *got)
	}
	if got := publicDescription(&doc); strings.Contains(*got, "poisoned") || !strings.Contains(*got, "Harvest feast.") {
		t.Errorf("description JSON kept the GM-only part: %q", *got)
	}
	if got := publicDescription(&plain); *got != plain {
		t.Errorf("plain text changed: %q", *got)
	}
}
