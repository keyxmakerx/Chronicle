// parent_selector_xss_test.go — regression pin for C-SEC-XSS-JSATTR-SWEEP-R1
// sink 1: a parent entity's free-text Name flowed verbatim into the
// parentSelector Alpine `x-data` expression (`selectedName: '%s'`), so any
// member who could rename an entity to `');<payload>//` and set it as another
// entity's parent planted stored JS that executed in the edit form of every
// member who opened the child. The fix routes the name through the entities
// plugin's jsEsc helper at the sink.
//
// Why the assertions look the way they do: templ HTML-escapes the attribute
// value with html.EscapeString, so every `'` renders as `&#39;` and the
// browser HTML-decodes it back before Alpine evaluates the string as JS. jsEsc
// prepends a backslash to the quote, and a backslash is NOT an HTML
// metacharacter, so it survives the round-trip. The rendered discriminator is
// therefore:
//   - fixed:      selectedName: &#39;\&#39;);alert(1)//&#39;   (backslash before the injected quote)
//   - vulnerable: selectedName: &#39;&#39;);alert(1)//&#39;    (two adjacent quote entities = literal closed)

package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestParentSelector_EscapesHostileParentName pins that a hostile parent name
// cannot break out of the selectedName JS string literal.
func TestParentSelector_EscapesHostileParentName(t *testing.T) {
	const payload = `');alert(1)//`
	parent := &Entity{ID: "11111111-1111-1111-1111-111111111111", Name: payload}

	var buf bytes.Buffer
	if err := parentSelector("camp-1", parent).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	html := buf.String()

	// The unescaped breakout — two adjacent quote entities immediately before
	// the injected payload — must NOT appear. (`query: ''` etc. render as
	// &#39;&#39; too, so the assertion is anchored on the full breakout run.)
	if strings.Contains(html, "&#39;&#39;);alert(1)") {
		t.Errorf("parentSelector let a hostile parent name break out of the selectedName literal; jsEsc missing at the sink\nrendered: %s", html)
	}
	// The escaped form — a backslash before the injected quote entity — MUST
	// appear, proving the value routed through jsEsc.
	if !strings.Contains(html, `\&#39;);alert(1)`) {
		t.Errorf("expected the injected quote to be backslash-escaped (jsEsc) in selectedName; not found\nrendered: %s", html)
	}
}

// TestParentSelector_LegitNameWithApostropheUnchanged pins that an ordinary
// name containing an apostrophe (e.g. "Bob's Tavern") still renders — escaped,
// not mangled or dropped — so the fix does not regress legitimate values.
func TestParentSelector_LegitNameWithApostropheUnchanged(t *testing.T) {
	parent := &Entity{ID: "22222222-2222-2222-2222-222222222222", Name: "Bob's Tavern"}

	var buf bytes.Buffer
	if err := parentSelector("camp-1", parent).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	html := buf.String()

	// The apostrophe is backslash-escaped and the surrounding text is intact.
	if !strings.Contains(html, `Bob\&#39;s Tavern`) {
		t.Errorf("legit name with apostrophe was not rendered as an escaped JS string\nrendered: %s", html)
	}
}

// TestParentSelector_NilParentRendersEmpty pins that the no-parent case (the
// common one) renders an empty selectedName without panicking.
func TestParentSelector_NilParentRendersEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := parentSelector("camp-1", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !strings.Contains(buf.String(), "selectedName: &#39;&#39;") {
		t.Errorf("nil parent should render an empty selectedName literal\nrendered: %s", buf.String())
	}
}
