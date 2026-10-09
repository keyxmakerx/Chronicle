package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestInitials(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"empty", "", "?"},
		{"blank", "   ", "?"},
		{"one word", "robin", "R"},
		{"two words", "robin hood", "RH"},
		{"three words uses first and last", "mary jane watson", "MW"},
		{"multi-byte letter is not cut", "élodie", "É"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Initials(tc.in); got != tc.want {
				t.Fatalf("Initials(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The box offers Remove only when there is a picture to remove, and every
// control is a plain inline handler (no sibling <script> to go missing after
// a swap).
func TestUserPictureBoxRender(t *testing.T) {
	tests := []struct {
		name       string
		avatarID   string
		wantRemove bool
	}{
		{"no picture shows the initials note", "", false},
		{"picture shows the remove link", "b7c17bb1-6563-462c-8b49-5b2e8bd57108", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := SetUserName(context.Background(), "Robin Hood")
			if tc.avatarID != "" {
				ctx = SetUserAvatarPath(ctx, tc.avatarID)
			}
			var buf bytes.Buffer
			if err := UserPictureBox().Render(ctx, &buf); err != nil {
				t.Fatal(err)
			}
			html := buf.String()
			if strings.Contains(html, "<script") {
				t.Fatal("picture box must not emit a <script> sibling")
			}
			if !strings.Contains(html, "Chronicle.PictureBox.toggle()") || !strings.Contains(html, `id="pk-warn"`) {
				t.Fatal("missing the toggle control or the warn bar")
			}
			removeLink := html[strings.Index(html, `id="pk-remove"`):]
			removeLink = removeLink[:strings.Index(removeLink, ">")]
			if gotHidden := strings.Contains(removeLink, "hidden"); gotHidden == tc.wantRemove {
				t.Fatalf("remove link tag %q: hidden = %v, want visible = %v", removeLink, gotHidden, tc.wantRemove)
			}
		})
	}
}
