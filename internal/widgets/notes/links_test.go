package notes

import (
	"reflect"
	"strings"
	"testing"
)

const (
	idA = "11111111-1111-1111-1111-111111111111"
	idB = "22222222-2222-2222-2222-222222222222"
	idP = "33333333-3333-3333-3333-333333333333"
)

func TestParseLinks(t *testing.T) {
	body := `<p>See <a class="note-link" data-note-id="` + idA + `" href="/x">Journal note</a> and ` +
		`<a data-mention-id="` + idP + `" href="/p">@Ashkeep Ruins</a>, again <a data-note-id="` + idA + `">Journal note</a>, ` +
		`<a data-note-id="not an id">x</a> <a href="https://example.com">web</a> <a data-note-id="` + idB + `">Journal note</a></p>`
	got := ParseLinks(body)
	want := []Link{
		{Kind: LinkNote, ID: idA},
		{Kind: LinkPage, ID: idP, Label: "Ashkeep Ruins"},
		{Kind: LinkNote, ID: idB},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseLinks = %+v\nwant %+v", got, want)
	}
	if ParseLinks("") != nil || ParseLinks("<p>plain</p>") != nil {
		t.Error("a body with no links has none")
	}
}

// TestPlainText_LabelsNoteLinksPerViewer: a note link reads as whatever the
// label function says for this viewer, never as its stored anchor text.
func TestPlainText_LabelsNoteLinksPerViewer(t *testing.T) {
	body := `<p>Under the keep: <a data-note-id="` + idA + `">Journal note</a>.</p><p>Also <a data-note-id="` + idB + `">Journal note</a></p>`
	visible := map[string]string{idA: "Thalrik Mourngrave"}
	got := PlainText(body, labelFrom(visible))
	if got != "Under the keep: Thalrik Mourngrave. Also a private note" {
		t.Errorf("PlainText = %q", got)
	}
	if strings.Contains(got, NoteLinkText) {
		t.Error("the stored anchor text must never reach plain text")
	}
}

func TestSnippets(t *testing.T) {
	if got := snippet("short text", 50); got != "short text" {
		t.Errorf("snippet kept short text as %q", got)
	}
	if got := snippet("one two three four five six", 12); got != "one two…" {
		t.Errorf("snippet = %q, want a word-boundary cut", got)
	}
	text := "Ärger in İstanbul: the lich Thalrik watches from below the old cistern"
	if got := matchSnippet(text, "THALRIK"); !strings.Contains(got, "Thalrik") {
		t.Errorf("matchSnippet = %q", got)
	}
	if matchSnippet(text, "dragon") != "" {
		t.Error("no match, no snippet")
	}
	if !containsFold("Session 12 — Into the Depths", "into THE") {
		t.Error("containsFold ignores case")
	}
}

func TestMentionLines(t *testing.T) {
	const a, b = "11111111-aaaa", "22222222-bbbb"
	tests := []struct {
		name string
		body string
		want map[string]string
	}{
		{"none", `<p>Nothing here</p>`, map[string]string{}},
		{
			"each page gets its own paragraph, first one wins",
			`<p>The party met <a data-mention-id="` + a + `">@Maudry</a> at the mill.</p>` +
				`<ul><li>Later <a data-mention-id="` + b + `">@Ansel</a> and <a data-mention-id="` + a + `">@Maudry</a> argued.</li></ul>`,
			map[string]string{a: "The party met @Maudry at the mill.", b: "Later @Ansel and @Maudry argued."},
		},
		{
			"note links keep their neutral text",
			`<p><a data-mention-id="` + a + `">@Maudry</a> read <a data-note-id="33333333-cccc">Journal note</a>.</p>`,
			map[string]string{a: "@Maudry read Journal note."},
		},
		{"bad id ignored", `<p><a data-mention-id="x%">@X</a></p>`, map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MentionLines(tt.body)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("%s = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}
