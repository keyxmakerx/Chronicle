package maps

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// The single-drawing and single-token reads must withhold exactly what the
// list reads withhold, or an id from elsewhere reads hidden content.
func TestDrawingVisibleTo(t *testing.T) {
	str := func(s string) *string { return &s }
	denyBob := str(`{"denied_users":["bob"]}`)
	onlyAnn := str(`{"allowed_users":["ann"]}`)
	broken := str(`{not json`)

	tests := []struct {
		name   string
		d      *Drawing
		role   int
		userID string
		want   bool
	}{
		{"player sees an everyone drawing", &Drawing{Visibility: "everyone"}, permissions.RolePlayer, "bob", true},
		{"player never sees dm_only", &Drawing{Visibility: "dm_only"}, permissions.RolePlayer, "bob", false},
		{"scribe never sees dm_only", &Drawing{Visibility: "dm_only"}, permissions.RoleScribe, "bob", false},
		{"owner sees dm_only", &Drawing{Visibility: "dm_only"}, permissions.RoleOwner, "own", true},
		{"denied player is refused", &Drawing{Visibility: "everyone", VisibilityRules: denyBob}, permissions.RolePlayer, "bob", false},
		{"other player passes a deny list", &Drawing{Visibility: "everyone", VisibilityRules: denyBob}, permissions.RolePlayer, "ann", true},
		{"anonymous refused by a deny list", &Drawing{Visibility: "everyone", VisibilityRules: denyBob}, permissions.RoleNone, "", false},
		{"allowlist admits its player", &Drawing{Visibility: "everyone", VisibilityRules: onlyAnn}, permissions.RolePlayer, "ann", true},
		{"allowlist refuses others", &Drawing{Visibility: "everyone", VisibilityRules: onlyAnn}, permissions.RolePlayer, "bob", false},
		{"owner bypasses rules", &Drawing{Visibility: "everyone", VisibilityRules: onlyAnn}, permissions.RoleOwner, "own", true},
		{"unparseable rules hide", &Drawing{Visibility: "everyone", VisibilityRules: broken}, permissions.RolePlayer, "ann", false},
		{"nil drawing", nil, permissions.RoleOwner, "own", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DrawingVisibleTo(tt.d, tt.role, tt.userID); got != tt.want {
				t.Fatalf("DrawingVisibleTo = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenVisibleTo(t *testing.T) {
	tests := []struct {
		name string
		tok  *Token
		role int
		want bool
	}{
		{"player sees a shown token", &Token{}, permissions.RolePlayer, true},
		{"player never sees a hidden token", &Token{IsHidden: true}, permissions.RolePlayer, false},
		{"scribe never sees a hidden token", &Token{IsHidden: true}, permissions.RoleScribe, false},
		{"owner sees a hidden token", &Token{IsHidden: true}, permissions.RoleOwner, true},
		{"nil token", nil, permissions.RoleOwner, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TokenVisibleTo(tt.tok, tt.role); got != tt.want {
				t.Fatalf("TokenVisibleTo = %v, want %v", got, tt.want)
			}
		})
	}
}
