// entity_permissions_ux_test.go — C-ENTITY-PERMISSIONS-UX. Pins the three
// presentation changes:
//
//	Part 1 — the entity card's single 3-state visibility badge (Everyone /
//	         DM-Only / Custom), Scribe+ gated (players never see it).
//	Part 2 — the edit-form permissions mount opts into the inline layout
//	         (data-layout="inline"); the JS behavior is covered by
//	         test/js/permissions_inline.test.mjs.
//	Part 3 — the read-only Category › Sub-category lineage in the edit form
//	         (with-parent and no-parent cases).
package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func renderEntityCard(t *testing.T, entity *Entity, role campaigns.Role) string {
	t.Helper()
	return renderEntityCardOnCampaign(t, entity, role, false)
}

// renderEntityCardOnCampaign renders a card on a campaign whose IsPublic is
// set explicitly. The glance's "everyone" wording depends on it — a logged-out
// reader can open pages of a public campaign and nothing at all of a private
// one — so a helper that hard-coded one of the two would let the other rot.
func renderEntityCardOnCampaign(t *testing.T, entity *Entity, role campaigns.Role, isPublic bool) string {
	t.Helper()
	cc := &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1", Name: "Test", IsPublic: isPublic},
		MemberRole: role,
	}
	var buf bytes.Buffer
	if err := EntityRow(entity, cc).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render row: %v", err)
	}
	return buf.String()
}

// Part 1 — a page players can't see carries the Director's amber on its row:
// the lock after the name and the leading edge (data-dm-only). A page everyone
// sees carries nothing, and a page shared with chosen players keeps the
// shield. The owner and Co-DMs see these; players never do (below).
func TestEntityRow_DirectorMark_States(t *testing.T) {
	cases := []struct {
		name       string
		entity     *Entity
		wantLock   bool
		wantShield bool
	}{
		{"everyone", &Entity{ID: "e1", Name: "Town", Visibility: VisibilityDefault, IsPrivate: false}, false, false},
		{"dm_only", &Entity{ID: "e2", Name: "Secret", Visibility: VisibilityDefault, IsPrivate: true}, true, false},
		{"hidden by its category", &Entity{ID: "e4", Name: "Plot", Visibility: VisibilityDefault, TypeDMOnly: true}, true, false},
		{"custom", &Entity{ID: "e3", Name: "Shared", Visibility: VisibilityCustom}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderEntityCard(t, tc.entity, campaigns.RoleOwner)
			if got := strings.Contains(html, "data-dm-only"); got != tc.wantLock {
				t.Errorf("data-dm-only present = %v, want %v", got, tc.wantLock)
			}
			if got := strings.Contains(html, "cb-dm"); got != tc.wantLock {
				t.Errorf("amber lock present = %v, want %v", got, tc.wantLock)
			}
			if got := strings.Contains(html, `data-visibility-badge="custom"`); got != tc.wantShield {
				t.Errorf("custom shield present = %v, want %v", got, tc.wantShield)
			}
			if strings.Contains(html, "fa-globe") {
				t.Errorf("a row no longer says Everyone with a globe")
			}
		})
	}
}

// Part 1 — players (below Scribe) get no mark on any row: no lock, no amber
// edge, no badge.
func TestEntityRow_DirectorMark_PlayerSeesNone(t *testing.T) {
	for _, e := range []*Entity{
		{ID: "e1", Name: "Town", Visibility: VisibilityDefault, IsPrivate: false},
		{ID: "e2", Name: "Secret", Visibility: VisibilityDefault, IsPrivate: true},
		{ID: "e3", Name: "Shared", Visibility: VisibilityCustom},
	} {
		html := renderEntityCard(t, e, campaigns.RolePlayer)
		for _, bad := range []string{"data-visibility-badge", "data-dm-only", "cb-dm"} {
			if strings.Contains(html, bad) {
				t.Errorf("player must not see %s on the row for %q", bad, e.Name)
			}
		}
	}
}

// Part 2 — the edit-form permissions mount opts into inline layout.
func TestEntityEditForm_PermissionsInlineLayout(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}, MemberRole: campaigns.RoleOwner}
	entity := &Entity{ID: "ent-1", CampaignID: "camp-1", Name: "Gandalf"}
	entityType := &EntityType{ID: 1, Name: "Character"}
	var buf bytes.Buffer
	if err := EntityEditFormComponent(cc, entity, entityType, nil, "csrf", "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, `data-widget="permissions"`) {
		t.Fatalf("edit form missing the permissions mount")
	}
	if !strings.Contains(html, `data-layout="inline"`) {
		t.Errorf("edit-form permissions mount must opt into data-layout=\"inline\"")
	}
}

// Part 3 — lineage line shows Parent › Type when the type has a parent.
func TestEntityEditForm_LineageWithParent(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}, MemberRole: campaigns.RoleOwner}
	entity := &Entity{ID: "ent-1", CampaignID: "camp-1", Name: "Waterdeep"}
	parent := "Location"
	entityType := &EntityType{ID: 2, Name: "City", ParentTypeName: &parent}
	var buf bytes.Buffer
	if err := EntityEditFormComponent(cc, entity, entityType, nil, "csrf", "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "data-type-lineage") {
		t.Fatalf("edit form missing the lineage line")
	}
	snip := lineageSnippet(html)
	if !strings.Contains(snip, "Location") || !strings.Contains(snip, "City") {
		t.Errorf("lineage should show 'Location › City'; got: %q", snip)
	}
	if !strings.Contains(snip, "fa-chevron-right") {
		t.Errorf("lineage with a parent should render the › separator; got: %q", snip)
	}
}

// Part 3 — lineage line shows just the type when there's no parent.
func TestEntityEditForm_LineageNoParent(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}, MemberRole: campaigns.RoleOwner}
	entity := &Entity{ID: "ent-1", CampaignID: "camp-1", Name: "Gandalf"}
	entityType := &EntityType{ID: 1, Name: "Character"} // no ParentTypeName
	var buf bytes.Buffer
	if err := EntityEditFormComponent(cc, entity, entityType, nil, "csrf", "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "data-type-lineage") {
		t.Fatalf("edit form missing the lineage line")
	}
	snip := lineageSnippet(html)
	if !strings.Contains(snip, "Character") {
		t.Errorf("lineage should show the type name 'Character'; got: %q", snip)
	}
	if strings.Contains(snip, "fa-chevron-right") {
		t.Errorf("lineage without a parent must not render a › separator; got: %q", snip)
	}
}

// lineageSnippet trims the rendered HTML around the lineage marker for nicer
// failure messages.
func lineageSnippet(html string) string {
	i := strings.Index(html, "data-type-lineage")
	if i < 0 {
		return html
	}
	end := i + 400
	if end > len(html) {
		end = len(html)
	}
	return html[i:end]
}
