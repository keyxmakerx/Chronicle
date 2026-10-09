package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

func foldTestCC(role campaigns.Role) *campaigns.CampaignContext {
	return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: role}
}

func renderTitle(t *testing.T, role campaigns.Role, userID string, entity *Entity) string {
	t.Helper()
	ctx := layouts.SetUserID(context.Background(), userID)
	var buf bytes.Buffer
	if err := blockTitle(foldTestCC(role), entity, "tok").Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestBlockTitle_EditNameFold(t *testing.T) {
	owner := "u-owner"
	label := "Hero"
	ent := &Entity{ID: "e1", CampaignID: "c1", Name: "Bren", TypeLabel: &label, OwnerUserID: &owner}

	cases := []struct {
		name        string
		role        campaigns.Role
		userID      string
		wantEdit    bool
		wantStruct  bool // descriptor + parent search in the fold
		wantHistory bool
	}{
		{"scribe gets the whole fold", campaigns.RoleScribe, "u-scribe", true, true, true},
		{"gm gets the whole fold", campaigns.RoleOwner, "u-gm", true, true, true},
		{"claimed player gets name only", campaigns.RolePlayer, owner, true, false, false},
		{"other player gets nothing", campaigns.RolePlayer, "u-other", false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderTitle(t, tc.role, tc.userID, ent)
			if got := strings.Contains(html, `data-edit-name`); got != tc.wantEdit {
				t.Errorf("edit button = %v, want %v", got, tc.wantEdit)
			}
			if got := strings.Contains(html, `id="page-name-fold"`); got != tc.wantEdit {
				t.Errorf("fold = %v, want %v", got, tc.wantEdit)
			}
			if got := strings.Contains(html, `data-parent-q`) && strings.Contains(html, `data-f="label"`); got != tc.wantStruct {
				t.Errorf("descriptor+parent search = %v, want %v", got, tc.wantStruct)
			}
			if got := strings.Contains(html, `page-history-slot`); got != tc.wantHistory {
				t.Errorf("history slot = %v, want %v", got, tc.wantHistory)
			}
		})
	}
}

// The old panel was an Alpine form with a raw parent id box that reloaded the
// page on save; none of that may come back.
func TestBlockTitle_NoOldPanel(t *testing.T) {
	ent := &Entity{ID: "e1", CampaignID: "c1", Name: "Bren"}
	html := renderTitle(t, campaigns.RoleScribe, "u", ent)
	for _, bad := range []string{"x-data", "x-show", "window.location.reload", "Parent entity ID", "Parent page ID"} {
		if strings.Contains(html, bad) {
			t.Errorf("rendered title still contains %q", bad)
		}
	}
	// The fold is told what is saved and where to send changes.
	for _, want := range []string{`data-endpoint="/campaigns/c1/entities/e1/metadata"`, `data-name="Bren"`, `data-search-endpoint="/campaigns/c1/entities/search"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestParentOfEntity(t *testing.T) {
	p := "p1"
	cases := []struct {
		name      string
		entity    *Entity
		ancestors []Entity
		want      pageParent
	}{
		{"no parent", &Entity{ID: "e"}, nil, pageParent{}},
		{"found by id, not position", &Entity{ID: "e", ParentID: &p}, []Entity{{ID: "other", Name: "Other"}, {ID: "p1", Name: "Parent"}}, pageParent{ID: "p1", Name: "Parent"}},
		{"hidden parent keeps its id", &Entity{ID: "e", ParentID: &p}, nil, pageParent{ID: "p1"}},
	}
	for _, tc := range cases {
		if got := parentOfEntity(tc.entity, tc.ancestors); got != tc.want {
			t.Errorf("%s: got %+v want %+v", tc.name, got, tc.want)
		}
	}
}
