package dmscreen

import (
	"strings"
	"testing"
)

func TestNoteNames(t *testing.T) {
	tests := []struct {
		name         string
		night        *NightView
		label, title string
	}{
		{"no night", nil, "DM Screen notes", "DM Screen notes"},
		{"blank night name", &NightView{Name: "  "}, "DM Screen notes", "DM Screen notes"},
		{"named night", &NightView{Name: "Session 12"}, "Kept with Session 12", "Session 12: DM notes"},
		{"name is trimmed", &NightView{Name: " Friday game "}, "Kept with Friday game", "Friday game: DM notes"},
		{"long name is cut", &NightView{Name: strings.Repeat("x", 400)}, "Kept with " + strings.Repeat("x", 150), strings.Repeat("x", 150) + ": DM notes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, ti := noteNames(tt.night)
			if l != tt.label || ti != tt.title {
				t.Errorf("got %q / %q", l, ti)
			}
			if len(ti) > 200 {
				t.Errorf("title over the notes limit: %d", len(ti))
			}
		})
	}
}

func TestPlainProseRoundTrip(t *testing.T) {
	tests := []struct{ name, text string }{
		{"empty", ""},
		{"one line", "hello"},
		{"lines and a blank", "one\n\ntwo"},
		{"markup stays text", "<b>bold</b> & more"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, htmlOut := proseFromPlain(tt.text)
			if got := plainFromProse(entry); got != tt.text {
				t.Errorf("round trip = %q, want %q", got, tt.text)
			}
			if strings.Contains(htmlOut, "<b>") {
				t.Errorf("html not escaped: %s", htmlOut)
			}
		})
	}
}

func TestPlainFromProse_RichContent(t *testing.T) {
	tests := []struct{ name, entry, want string }{
		{"blank", "", ""},
		{"not json", "nope", ""},
		{"heading and paragraph", `{"type":"doc","content":[{"type":"heading","content":[{"type":"text","text":"Plan"}]},{"type":"paragraph","content":[{"type":"text","text":"Bold","marks":[{"type":"bold"}]},{"type":"text","text":" move"}]}]}`, "Plan\nBold move"},
		{"bullet list", `{"type":"doc","content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"b"}]}]}]}]}`, "a\nb"},
		{"hard break", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x"},{"type":"hardBreak"},{"type":"text","text":"y"}]}]}`, "x\ny"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := plainFromProse(tt.entry); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
