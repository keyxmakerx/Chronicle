package pages

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestErrorPage pins what each error page says: a not-found page covers a
// hidden page and a missing one in the same words and never repeats the
// error's own text, and only signing in is drawn as a button.
func TestErrorPage(t *testing.T) {
	tests := []struct {
		name    string
		code    int
		message string
		want    []string
		notWant []string
	}{
		{
			name: "not found hides which it is", code: 404, message: "entity not found",
			want:    []string{"This page isn&#39;t shared with you, or doesn&#39;t exist.", "ask them to check it", `class="quiet-word"`},
			notWant: []string{"entity not found", "has been moved", "btn-primary"},
		},
		{
			name: "sign in is the one command", code: 401, message: "authentication required",
			want: []string{`href="/login" class="btn-primary"`, "Create an account"},
		},
		{
			name: "server error keeps its message", code: 500, message: "An unexpected error occurred",
			want:    []string{"An unexpected error occurred", "Try again in a moment"},
			notWant: []string{"bg-red-500", "btn-secondary"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := ErrorPage(tc.code, tc.message).Render(context.Background(), &buf); err != nil {
				t.Fatal(err)
			}
			// Only the page's own part; the base layout draws other controls.
			html := buf.String()
			if i := strings.Index(html, `id="main-content"`); i >= 0 {
				html = html[i:]
			}
			if j := strings.Index(html, "</main>"); j >= 0 {
				html = html[:j]
			}
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(html, w) {
					t.Errorf("should not contain %q", w)
				}
			}
		})
	}
}
