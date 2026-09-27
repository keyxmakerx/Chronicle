package sanitize

import (
	"strings"
	"testing"
)

// TestHTML_KeepsNoteLinkIDs: a [[note]] link survives sanitizing with its
// id, and a data-note-id that is not id-shaped is dropped rather than kept
// for readers to resolve.
func TestHTML_KeepsNoteLinkIDs(t *testing.T) {
	const id = "11111111-2222-3333-4444-555555555555"
	got := HTML(`<p><a class="note-link" data-note-id="` + id + `" href="/campaigns/c/journal/` + id + `">Journal note</a></p>`)
	if !strings.Contains(got, `data-note-id="`+id+`"`) {
		t.Errorf("a note link's id must survive: %s", got)
	}

	got = HTML(`<a data-note-id="&quot; onclick=&quot;x" href="/x">bad</a><a data-note-id="../../etc">bad</a>`)
	if strings.Contains(got, "data-note-id") {
		t.Errorf("a malformed note id must be dropped: %s", got)
	}
	if strings.Contains(got, "onclick") {
		t.Errorf("no handler may survive: %s", got)
	}
}
