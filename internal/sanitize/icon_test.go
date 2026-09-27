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
		{"attribute break", `fa-x" onmouseover="alert(1)`, false},
		{"angle brackets", "fa-<b>", false},
		{"closing tag", "fa-x></i><img src=x>", false},
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
		{"angle refused", "<script>", "", true},
		{"prefix alone refused", "fa-", "", true},
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
		{"bad replaced and noted", `fa-x"><img>`, "fa-circle", true},
		{"uppercase replaced", "FA-DRAGON", "fa-circle", true},
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
