// dm_only_marks_test.go pins who sees the DM-only marks: the lock on a
// sidebar row and the label beside the page name. A player must get no
// element at all, since even a hidden one would hint at secrets.
package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func marksContext(role campaigns.Role, granted bool) *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1", Name: "C"},
		MemberRole:  role,
		IsDmGranted: granted,
	}
}

func TestDMOnlyTitleLabel(t *testing.T) {
	tests := []struct {
		name       string
		cc         *campaigns.CampaignContext
		entity     Entity
		wantLabel  bool
		wantHidden bool
	}{
		{"owner, DM-only page", marksContext(campaigns.RoleOwner, false), Entity{ID: "e", IsPrivate: true}, true, false},
		{"co-DM, DM-only page", marksContext(campaigns.RolePlayer, true), Entity{ID: "e", IsPrivate: true}, true, false},
		{"owner, page in a DM-only category", marksContext(campaigns.RoleOwner, false), Entity{ID: "e", TypeDMOnly: true}, true, false},
		{"owner, everyone page", marksContext(campaigns.RoleOwner, false), Entity{ID: "e"}, true, true},
		{"owner, custom page", marksContext(campaigns.RoleOwner, false), Entity{ID: "e", IsPrivate: true, Visibility: VisibilityCustom}, true, true},
		{"player", marksContext(campaigns.RolePlayer, false), Entity{ID: "e", IsPrivate: true}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := dmOnlyTitleLabel(tt.cc, &tt.entity).Render(context.Background(), &buf); err != nil {
				t.Fatalf("render: %v", err)
			}
			html := buf.String()
			if got := strings.Contains(html, "data-dm-only-label"); got != tt.wantLabel {
				t.Fatalf("label rendered = %v, want %v: %s", got, tt.wantLabel, html)
			}
			if !tt.wantLabel {
				return
			}
			if got := strings.Contains(html, " hidden"); got != tt.wantHidden {
				t.Errorf("hidden = %v, want %v: %s", got, tt.wantHidden, html)
			}
		})
	}
}

func TestSidebarEntityList_DMOnlyLock(t *testing.T) {
	results := []Entity{
		{ID: "secret", Name: "Tobias", EntityTypeID: 7, IsPrivate: true},
		{ID: "open", Name: "Pell", EntityTypeID: 7},
		{ID: "in-dm-cat", Name: "Adventure", EntityTypeID: 7, IsPrivate: true, TypeDMOnly: true},
	}
	tests := []struct {
		name  string
		cc    *campaigns.CampaignContext
		locks int
	}{
		{"owner", marksContext(campaigns.RoleOwner, false), 1},
		{"co-DM", marksContext(campaigns.RolePlayer, true), 1},
		{"player", marksContext(campaigns.RolePlayer, false), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := SidebarEntityList(results, nil, len(results), 7, tt.cc, nil).Render(context.Background(), &buf); err != nil {
				t.Fatalf("render: %v", err)
			}
			if got := strings.Count(buf.String(), "tree-lock"); got != tt.locks {
				t.Errorf("locks = %d, want %d", got, tt.locks)
			}
		})
	}
}
