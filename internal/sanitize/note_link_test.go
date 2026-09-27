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

// TestStripSecretsJSON_DropsMarkedNoteLinks: a [[note]] link inside a GM
// secret carries the secret mark itself (it is an inline node, not text),
// and must leave with the secret, or a player's copy of the page shows what
// the secret links to.
func TestStripSecretsJSON_DropsMarkedNoteLinks(t *testing.T) {
	const id = "11111111-2222-3333-4444-555555555555"
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[` +
		`{"type":"text","text":"Open. "},` +
		`{"type":"text","text":"GM: ","marks":[{"type":"secret"}]},` +
		`{"type":"noteLink","attrs":{"noteId":"` + id + `"},"marks":[{"type":"secret"}]},` +
		`{"type":"noteLink","attrs":{"noteId":"22222222-2222-3333-4444-555555555555"}}]}]}`
	got := StripSecretsJSON(doc)
	if strings.Contains(got, id) || strings.Contains(got, "GM: ") {
		t.Errorf("the secret and the link inside it must go: %s", got)
	}
	if !strings.Contains(got, "Open. ") || !strings.Contains(got, "22222222-2222-3333-4444-555555555555") {
		t.Errorf("everything outside the secret stays: %s", got)
	}
}
