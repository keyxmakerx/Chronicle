package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The picture's Change chip must be drawn for people who can edit, on the
// picture itself rather than behind a hover, and not drawn for everyone else.
func TestBlockImage_ChangeChip(t *testing.T) {
	path := "media/pic.png"
	tests := []struct {
		name     string
		role     campaigns.Role
		image    *string
		wantChip bool
		wantAdd  bool
	}{
		{"scribe with picture sees the chip", campaigns.RoleScribe, &path, true, false},
		{"player with picture sees none", campaigns.RolePlayer, &path, false, false},
		{"scribe without picture sees Add Image", campaigns.RoleScribe, nil, false, true},
		{"player without picture sees none", campaigns.RolePlayer, nil, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: tt.role}
			var buf bytes.Buffer
			if err := blockImage(cc, &Entity{ID: "e1", Name: "Tyne", ImagePath: tt.image}, nil, "csrf").Render(context.Background(), &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if got := strings.Contains(out, "data-pick"); got != tt.wantChip {
				t.Errorf("chip present = %v, want %v\n%s", got, tt.wantChip, out)
			}
			if got := strings.Contains(out, "Add Image"); got != tt.wantAdd {
				t.Errorf("Add Image present = %v, want %v", got, tt.wantAdd)
			}
			if strings.Contains(out, "group-hover") {
				t.Error("the upload control must not be hover-only")
			}
		})
	}
}

// "Make token" is offered only where the token has somewhere to go: a page
// with a picture whose type, or the page itself, declares the token field,
// and only to people who can edit it.
func TestBlockImage_MakeTokenChip(t *testing.T) {
	path := "media/pic.png"
	creature := &EntityType{Fields: []FieldDefinition{{Key: "stamina"}, {Key: TokenFieldKey}}}
	plain := &EntityType{Fields: []FieldDefinition{{Key: "stamina"}}}
	tests := []struct {
		name   string
		role   campaigns.Role
		image  *string
		et     *EntityType
		over   *FieldOverrides
		wantMk bool
	}{
		{"scribe, creature with picture", campaigns.RoleScribe, &path, creature, nil, true},
		{"player, creature with picture", campaigns.RolePlayer, &path, creature, nil, false},
		{"scribe, creature without picture", campaigns.RoleScribe, nil, creature, nil, false},
		{"scribe, type without the field", campaigns.RoleScribe, &path, plain, nil, false},
		{"scribe, no type", campaigns.RoleScribe, &path, nil, nil, false},
		{"scribe, field added on the page", campaigns.RoleScribe, &path, plain, &FieldOverrides{Added: []FieldDefinition{{Key: TokenFieldKey}}}, true},
		{"scribe, field hidden on the page", campaigns.RoleScribe, &path, creature, &FieldOverrides{Hidden: []string{TokenFieldKey}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: tt.role}
			var buf bytes.Buffer
			e := &Entity{ID: "e1", Name: "Tyne", ImagePath: tt.image, FieldOverrides: tt.over}
			if err := blockImage(cc, e, tt.et, "csrf").Render(context.Background(), &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if got := strings.Contains(out, "data-token-maker"); got != tt.wantMk {
				t.Errorf("Make token present = %v, want %v\n%s", got, tt.wantMk, out)
			}
			if tt.wantMk {
				for _, want := range []string{`data-campaign-id="camp-1"`, `data-entity-id="e1"`, `data-name="Tyne"`, "data-art=", "data-pick"} {
					if !strings.Contains(out, want) {
						t.Errorf("missing %s\n%s", want, out)
					}
				}
			}
		})
	}
}
