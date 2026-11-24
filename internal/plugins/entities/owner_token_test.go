package entities

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// A claimed owner may set or clear their character's token picture, and only
// with a media id; another player, free text and a GM-only token are refused.
func TestAuthorizeFieldsWrite_OwnerToken(t *testing.T) {
	owner := "u1"
	ent := &Entity{CampaignID: "c1", OwnerUserID: &owner}
	tokenField := []FieldDefinition{{Key: TokenFieldKey}}
	id := "0b6f2f8e-4a52-4c3e-9d0a-2f1f6c2b7a10"
	cases := []struct {
		name   string
		uid    string
		patch  map[string]any
		fields []FieldDefinition
		ok     bool
	}{
		{"owner sets a media id", "u1", map[string]any{TokenFieldKey: id}, tokenField, true},
		{"owner clears it", "u1", map[string]any{TokenFieldKey: nil}, tokenField, true},
		{"free text", "u1", map[string]any{TokenFieldKey: "https://evil.example/x.png"}, tokenField, false},
		{"upper-case id", "u1", map[string]any{TokenFieldKey: "0B6F2F8E-4A52-4C3E-9D0A-2F1F6C2B7A10"}, tokenField, false},
		{"not a string", "u1", map[string]any{TokenFieldKey: 3}, tokenField, false},
		{"type has no token field", "u1", map[string]any{TokenFieldKey: id}, []FieldDefinition{{Key: "kit"}}, false},
		{"GM-only token field", "u1", map[string]any{TokenFieldKey: id}, []FieldDefinition{{Key: TokenFieldKey, GMOnly: true}}, false},
		{"another player", "u2", map[string]any{TokenFieldKey: id}, tokenField, false},
	}
	for _, tc := range cases {
		err := authorizeFieldsWrite(campaigns.RolePlayer, ent, tc.uid, "c1", false, tc.patch, tc.fields, nil)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok=%v", tc.name, err, tc.ok)
		}
	}
}
