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
			if err := blockImage(cc, &Entity{ID: "e1", Name: "Tyne", ImagePath: tt.image}, "csrf").Render(context.Background(), &buf); err != nil {
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
