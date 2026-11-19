package layouts

import (
	"context"
	"strings"
	"testing"
)

// The password check is a native dialog opened with showModal(), so it sits in
// the top layer above any other open dialog and Escape cancels it. A click
// outside must not cancel it: the typed password would be lost.
func TestReauthModalIsANativeDialog(t *testing.T) {
	var buf strings.Builder
	if err := ReauthModal().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render ReauthModal: %v", err)
	}
	html := buf.String()

	tests := []struct {
		name    string
		want    string
		present bool
	}{
		{"is a dialog element", `<dialog id="reauth-modal"`, true},
		{"opens as a modal", "showModal()", true},
		{"shared dialog motion", "dlg-modal", true},
		{"no outside-click cancel", "click.outside", false},
		{"no raw backdrop", "bg-black/50", false},
		{"no palette red", "text-red-", false},
		{"no stock shadow", "shadow-xl", false},
		{"title", "Confirm it's you", true},
		{"wrong password", "That password isn", true},
		{"locked out", "Too many tries. Wait 15 minutes, then try again.", true},
		{"old title", "Confirm Your Identity", false},
		{"old apology", "Please try again", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Contains(html, tt.want); got != tt.present {
				t.Errorf("contains %q = %v, want %v", tt.want, got, tt.present)
			}
		})
	}
}
