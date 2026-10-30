package sanitize

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const (
	seenID   = "11111111-1111-4111-8111-111111111111"
	hiddenID = "22222222-2222-4222-8222-222222222222"
)

func keepSeen(id string) bool { return id == seenID }

func mentionHTML(id, inner string) string {
	return `<a data-mention-id="` + id + `" href="/campaigns/c/entities/` + id + `" data-entity-preview="/campaigns/c/entities/` + id + `/preview">` + inner + `</a>`
}

// A mention of a page the reader can't open loses its name, address and
// preview link; one they can open is untouched.
func TestHideMentions_HTML(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		want  string
		count int
	}{
		{"hidden", `<p>Met ` + mentionHTML(hiddenID, "Duke Vampire") + ` today</p>`, `<p>Met ` + HiddenMentionText + ` today</p>`, 1},
		{"seen kept", `<p>` + mentionHTML(seenID, "Bram") + `</p>`, `<p>` + mentionHTML(seenID, "Bram") + `</p>`, 0},
		{"nested formatting", `<p>` + mentionHTML(hiddenID, "<strong>Duke</strong> <a>x</a> Vampire") + `!</p>`, `<p>` + HiddenMentionText + `!</p>`, 1},
		{"both", mentionHTML(seenID, "Bram") + ` and ` + mentionHTML(hiddenID, "Duke"), mentionHTML(seenID, "Bram") + ` and ` + HiddenMentionText, 1},
		{"unclosed hides to end", `<p>` + `<a data-mention-id="` + hiddenID + `">Duke</p><p>rest`, `<p>` + HiddenMentionText, 1},
		{"plain link untouched", `<p><a href="https://x">Duke</a></p>`, `<p><a href="https://x">Duke</a></p>`, 0},
		{"attribute text is not a mention", `<p title='data-mention-id="` + hiddenID + `"'>x</p>`, `<p title='data-mention-id="` + hiddenID + `"'>x</p>`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, n := HideMentions(tt.in, keepSeen)
			if got != tt.want || n != tt.count {
				t.Errorf("HideMentions(%q) = %q, %d; want %q, %d", tt.in, got, n, tt.want, tt.count)
			}
			if tt.count > 0 && (strings.Contains(got, "Duke") || strings.Contains(got, hiddenID)) {
				t.Errorf("hidden page's name or id survived: %q", got)
			}
		})
	}
}

func TestHideMentions_JSON(t *testing.T) {
	link := func(id string) string {
		return `{"type":"link","attrs":{"href":"/campaigns/c/entities/` + id + `","data-mention-id":"` + id + `"}}`
	}
	doc := func(content string) string {
		return `{"type":"doc","content":[{"type":"paragraph","content":[` + content + `]}]}`
	}
	tests := []struct {
		name  string
		in    string
		want  string
		count int
	}{
		{
			"hidden",
			doc(`{"type":"text","text":"Met "},{"type":"text","text":"Duke","marks":[` + link(hiddenID) + `]}`),
			doc(`{"type":"text","text":"Met "},{"text":"a hidden page","type":"text"}`),
			1,
		},
		{
			"split over pieces keeps the first piece's style",
			doc(`{"type":"text","text":"Duke ","marks":[{"type":"bold"},` + link(hiddenID) + `]},{"type":"text","text":"Vampire","marks":[` + link(hiddenID) + `]},{"type":"text","text":"."}`),
			doc(`{"marks":[{"type":"bold"}],"text":"a hidden page","type":"text"},{"type":"text","text":"."}`),
			1,
		},
		{
			"two mentions side by side stay two",
			doc(`{"type":"text","text":"A","marks":[` + link(hiddenID) + `]},{"type":"text","text":" "},{"type":"text","text":"B","marks":[` + link(hiddenID) + `]}`),
			doc(`{"text":"a hidden page","type":"text"},{"type":"text","text":" "},{"text":"a hidden page","type":"text"}`),
			2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, n := HideMentions(tt.in, keepSeen)
			if n != tt.count || !sameJSON(t, got, tt.want) {
				t.Errorf("HideMentions =\n %s, %d\nwant\n %s, %d", got, n, tt.want, tt.count)
			}
		})
	}

	seen := doc(`{"type":"text","text":"Bram","marks":[` + link(seenID) + `]}`)
	if got, n := HideMentions(seen, keepSeen); got != seen || n != 0 {
		t.Errorf("a visible mention must come back byte for byte, got %q, %d", got, n)
	}
}

func TestMentionIDs(t *testing.T) {
	in := mentionHTML(hiddenID, "a") + mentionHTML(seenID, "b") + mentionHTML(hiddenID, "c")
	if got := MentionIDs(in); !reflect.DeepEqual(got, []string{hiddenID, seenID}) {
		t.Errorf("MentionIDs = %v", got)
	}
	if got := MentionIDs("<p>none</p>"); got != nil {
		t.Errorf("MentionIDs(no mentions) = %v, want nil", got)
	}
}

func sameJSON(t *testing.T, a, b string) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal([]byte(a), &x); err != nil {
		t.Fatalf("bad JSON %q: %v", a, err)
	}
	if err := json.Unmarshal([]byte(b), &y); err != nil {
		t.Fatalf("bad JSON %q: %v", b, err)
	}
	return reflect.DeepEqual(x, y)
}

func TestDropMentionText(t *testing.T) {
	in := `<p>Met ` + mentionHTML(hiddenID, "<em>Duke</em>") + ` and ` + mentionHTML(seenID, "Bram") + `.</p>`
	if got, want := DropMentionText(in), `<p>Met  and .</p>`; got != want {
		t.Errorf("DropMentionText = %q, want %q", got, want)
	}
	if in := `<p>plain</p>`; DropMentionText(in) != in {
		t.Error("text with no mention changed")
	}
}
