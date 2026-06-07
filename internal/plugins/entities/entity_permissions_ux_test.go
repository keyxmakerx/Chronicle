// entity_permissions_ux_test.go — C-ENTITY-PERMISSIONS-UX. Pins the three
// presentation changes:
//   Part 1 — the entity card's single 3-state visibility badge (Everyone /
//            DM-Only / Custom), Scribe+ gated (players never see it).
//   Part 2 — the edit-form permissions mount opts into the inline layout
//            (data-layout="inline"); the JS behavior is covered by
//            test/js/permissions_inline.test.mjs.
//   Part 3 — the read-only Category › Sub-category lineage in the edit form
//            (with-parent and no-parent cases).
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
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}, MemberRole: role}
	var buf bytes.Buffer
	if err := EntityCard(entity, cc).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render card: %v", err)
	}
	return buf.String()
}

// Part 1 — each visibility state renders its distinct badge for Scribe+.
func TestEntityCard_VisibilityBadge_States(t *testing.T) {
	cases := []struct {
		name       string
		entity     *Entity
		wantBadge  string // data-visibility-badge value
		wantIcon   string
		wantTitleX string // a fragment of the title text
	}{
		{
			name:       "everyone",
			entity:     &Entity{ID: "e1", Name: "Town", Visibility: VisibilityDefault, IsPrivate: false},
			wantBadge:  `data-visibility-badge="everyone"`,
			wantIcon:   "fa-globe",
			wantTitleX: "Everyone",
		},
		{
			name:       "dm_only",
			entity:     &Entity{ID: "e2", Name: "Secret", Visibility: VisibilityDefault, IsPrivate: true},
			wantBadge:  `data-visibility-badge="dm_only"`,
			wantIcon:   "fa-lock",
			wantTitleX: "DM-Only",
		},
		{
			name:       "custom",
			entity:     &Entity{ID: "e3", Name: "Shared", Visibility: VisibilityCustom},
			wantBadge:  `data-visibility-badge="custom"`,
			wantIcon:   "fa-shield-halved",
			wantTitleX: "Custom",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderEntityCard(t, tc.entity, campaigns.RoleScribe)
			for _, want := range []string{tc.wantBadge, tc.wantIcon, tc.wantTitleX} {
				if !strings.Contains(html, want) {
					t.Errorf("%s badge missing %q", tc.name, want)
				}
			}
			// Exactly one badge slot — the other two states must be absent.
			for _, other := range []string{`data-visibility-badge="everyone"`, `data-visibility-badge="dm_only"`, `data-visibility-badge="custom"`} {
				if other != tc.wantBadge && strings.Contains(html, other) {
					t.Errorf("%s: unexpected second badge %q", tc.name, other)
				}
			}
		})
	}
}

// Part 1 — players (below Scribe) never see the badge.
func TestEntityCard_VisibilityBadge_PlayerHidden(t *testing.T) {
	for _, e := range []*Entity{
		{ID: "e1", Name: "Town", Visibility: VisibilityDefault, IsPrivate: false},
		{ID: "e2", Name: "Secret", Visibility: VisibilityDefault, IsPrivate: true},
		{ID: "e3", Name: "Shared", Visibility: VisibilityCustom},
	} {
		html := renderEntityCard(t, e, campaigns.RolePlayer)
		if strings.Contains(html, "data-visibility-badge") {
			t.Errorf("player must not see any visibility badge; got one for %q", e.Name)
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
