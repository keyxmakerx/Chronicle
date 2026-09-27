package sanitize

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestIsIconName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		ok   bool
	}{
		// Accepted.
		{"simple", "fa-book", true},
		{"hyphenated", "fa-map-location-dot", true},
		{"digits", "fa-dice-d20", true},
		{"at cap", "fa-" + strings.Repeat("a", MaxIconLength-3), true},

		// Rejected.
		{"empty", "", false},
		{"prefix alone", "fa-", false},
		{"no prefix", "book", false},
		{"style prefix", "fa-solid fa-book", false},
		{"space", "fa-book extra", false},
		{"leading space", " fa-book", false},
		{"trailing newline", "fa-book\n", false},
		{"tab", "fa-\tbook", false},
		{"uppercase", "fa-Book", false},
		{"uppercase prefix", "FA-book", false},
		{"double quote", `fa-book"`, false},
		{"single quote", "fa-book'", false},
		{"quote inside", `fa-x" data-y="z`, false},
		{"angle brackets", "fa-<b>", false},
		{"angle brackets inside", "fa-x></i><b>z</b>", false},
		{"ampersand", "fa-a&b", false},
		{"underscore", "fa-a_b", false},
		{"slash", "fa-a/b", false},
		{"non-ascii", "fa-é", false},
		{"emoji", "⭐", false},
		{"over cap", "fa-" + strings.Repeat("a", MaxIconLength-2), false},
		{"very long", "fa-" + strings.Repeat("a", 10000), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsIconName(tt.in); got != tt.ok {
				t.Errorf("IsIconName(%q) = %v, want %v", tt.in, got, tt.ok)
			}
		})
	}
}

func TestValidateIcon(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"valid", "fa-book", "fa-book", false},
		{"trimmed", "  fa-book  ", "fa-book", false},
		{"empty passes through", "", "", false},
		{"whitespace becomes empty", "   ", "", false},
		{"quote refused", `fa-x" onclick="y`, "", true},
		{"angle refused", "<b>", "", true},
		{"prefix alone refused", "fa-", "", true},

		// Style-prefixed names keep only the bare name.
		{"fa-solid prefix", "fa-solid fa-circle", "fa-circle", false},
		{"fas prefix", "fas fa-circle", "fa-circle", false},
		{"fa-regular prefix", "fa-regular fa-circle", "fa-circle", false},
		{"far prefix", "far fa-circle", "fa-circle", false},
		{"fa prefix", "fa fa-circle", "fa-circle", false},
		{"inner whitespace collapsed", "fa-solid  fa-circle", "fa-circle", false},
		{"tab and padding", " \tfa-solid\tfa-circle \n", "fa-circle", false},
		{"prefixed name at cap", "fa-solid fa-" + strings.Repeat("a", MaxIconLength-3), "fa-" + strings.Repeat("a", MaxIconLength-3), false},

		// Still refused after normalising.
		{"uppercase style token", "FA-SOLID fa-circle", "", true},
		{"brands style", "fa-brands fa-github", "", true},
		{"fab style", "fab fa-github", "", true},
		{"extra class", "fa-solid fa-circle extra", "", true},
		{"two names", "fa-circle fa-book", "", true},
		{"two style tokens", "fa-solid fas fa-circle", "", true},
		{"short style token alone", "fas", "", true},
		{"prefixed bad name", `fa-solid fa-x"`, "", true},
		{"prefixed name over cap", "fa-solid fa-" + strings.Repeat("a", MaxIconLength-2), "", true},
		{"over raw bound", "fa-solid" + strings.Repeat(" ", maxIconInputLength) + "fa-circle", "", true},
		{"whitespace over raw bound", strings.Repeat(" ", maxIconInputLength+1), "", true},
		{"very long", "fa-" + strings.Repeat("a", 1<<20), "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateIcon(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateIcon(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ValidateIcon(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if err != nil {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
					t.Errorf("ValidateIcon(%q) error = %v, want a 400 AppError", tt.in, err)
				}
			}
		})
	}
}

func TestIconOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		in           string
		want         string
		wantReplaced bool
	}{
		{"valid kept", "fa-dragon", "fa-dragon", false},
		{"trimmed", " fa-dragon ", "fa-dragon", false},
		{"empty gets default, not noted", "", "fa-circle", false},
		{"bad replaced and noted", `fa-x"><b>`, "fa-circle", true},
		{"uppercase replaced", "FA-DRAGON", "fa-circle", true},
		{"style prefix normalised", "fa-solid fa-dragon", "fa-dragon", false},
		{"brands replaced", "fa-brands fa-github", "fa-circle", true},
		{"extra class replaced", "fa-solid fa-dragon extra", "fa-circle", true},
		{"over raw bound replaced", strings.Repeat(" ", maxIconInputLength+1), "fa-circle", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, replaced := IconOrDefault(tt.in, "fa-circle")
			if got != tt.want || replaced != tt.wantReplaced {
				t.Errorf("IconOrDefault(%q) = (%q, %v), want (%q, %v)", tt.in, got, replaced, tt.want, tt.wantReplaced)
			}
		})
	}
}
