// sidebar_list_test.go pins that SidebarEntityList advertises the drilled
// category's own typeID via data-entity-type-id, not results[0].EntityTypeID.
// A drilled category rolls up its sub-type entities into the listing
// (expandTypeIDsForListing), so results[0] can be a SUB-type; sidebar_tree.js
// reads that attribute when creating an empty folder, and a sub-type-scoped
// folder vanishes on the next refresh (SearchAPI reloads folder nodes via
// ListByType(categoryTypeID), an exact match), orphaning the dropped
// entities.
package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func renderSidebarList(t *testing.T, results []Entity, nodes []SidebarNode, typeID int) string {
	t.Helper()
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "C"}, MemberRole: campaigns.RoleOwner}
	var buf bytes.Buffer
	if err := SidebarEntityList(results, nodes, len(results), typeID, cc, nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	return buf.String()
}

// TestSidebarEntityList_AdvertisesCategoryTypeNotSubType is the core pin: when
// the first listed entity is a sub-type (99) but the drilled category is 7, the
// tree must advertise 7 so a new empty folder is created (and reloaded) under the
// category — never under the rolled-up sub-type, which would make it disappear.
func TestSidebarEntityList_AdvertisesCategoryTypeNotSubType(t *testing.T) {
	results := []Entity{
		{ID: "e-1", Name: "Sub-typed page", EntityTypeID: 99}, // rolled-up sub-type row first
		{ID: "e-2", Name: "Parent page", EntityTypeID: 7},
	}

	html := renderSidebarList(t, results, nil, 7)

	if !strings.Contains(html, `data-entity-type-id="7"`) {
		t.Errorf("tree must advertise the drilled category type (7); got:\n%s", html)
	}
	if strings.Contains(html, `data-entity-type-id="99"`) {
		t.Errorf("tree must NOT advertise the rolled-up sub-type (99) — that scopes a new folder to a type ListByType(7) never returns:\n%s", html)
	}
}

// TestSidebarEntityList_FallsBackToRowTypeWhenNoCategoryType keeps the defensive
// fallback: if no category typeID is supplied (typeID <= 0), the old derivation
// from the first row still applies so the attribute is never simply absent.
func TestSidebarEntityList_FallsBackToRowTypeWhenNoCategoryType(t *testing.T) {
	results := []Entity{{ID: "e-1", Name: "Page", EntityTypeID: 42}}

	html := renderSidebarList(t, results, nil, 0)

	if !strings.Contains(html, `data-entity-type-id="42"`) {
		t.Errorf("with no category type, fall back to the first row's type (42); got:\n%s", html)
	}
}

// TestSidebarEntityList_NodesOnlyUsesCategoryType pins that a category holding
// only folder nodes (no entities yet) still advertises the category type, so a
// folder created there is scoped correctly too.
func TestSidebarEntityList_NodesOnlyUsesCategoryType(t *testing.T) {
	nodes := []SidebarNode{{ID: "n-1", Name: "Folder", EntityTypeID: 7}}

	html := renderSidebarList(t, nil, nodes, 7)

	if !strings.Contains(html, `data-entity-type-id="7"`) {
		t.Errorf("nodes-only listing must still advertise the category type (7); got:\n%s", html)
	}
}

// TestSidebarEntityList_NoStrayElseAttribute guards against the templ
// conditional-attribute quirk that emitted a literal "else" between chained
// branches. The single resolved attribute must be the only thing there.
func TestSidebarEntityList_NoStrayElseAttribute(t *testing.T) {
	html := renderSidebarList(t, []Entity{{ID: "e-1", Name: "Page", EntityTypeID: 7}}, nil, 7)
	if strings.Contains(html, " else") {
		t.Errorf("rendered tree tag must not contain a stray 'else' token:\n%s", html)
	}
}

// TestSidebarTreeType covers the resolver directly: category type wins; the
// first row and then the first node are fallbacks only when no category type is
// given; zero when nothing is available.
func TestSidebarTreeType(t *testing.T) {
	cases := []struct {
		name    string
		typeID  int
		results []Entity
		nodes   []SidebarNode
		want    int
	}{
		{"category type wins over sub-type row", 7, []Entity{{EntityTypeID: 99}}, nil, 7},
		{"falls back to first row", 0, []Entity{{EntityTypeID: 42}}, nil, 42},
		{"falls back to first node", 0, nil, []SidebarNode{{EntityTypeID: 5}}, 5},
		{"zero when nothing available", 0, nil, nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sidebarTreeType(tc.typeID, tc.results, tc.nodes); got != tc.want {
				t.Errorf("sidebarTreeType(%d, %v, %v) = %d, want %d", tc.typeID, tc.results, tc.nodes, got, tc.want)
			}
		})
	}
}

// TestSidebarEntityList_RowMenuFlags pins what the tree tells the row menu:
// edit actions from Scribe up, folder delete for the Owner only, matching the
// routes those actions call.
func TestSidebarEntityList_RowMenuFlags(t *testing.T) {
	cases := []struct {
		role         campaigns.Role
		edit, delete bool
	}{
		{campaigns.RolePlayer, false, false},
		{campaigns.RoleScribe, true, false},
		{campaigns.RoleOwner, true, true},
	}
	for _, tc := range cases {
		cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "C"}, MemberRole: tc.role}
		var buf bytes.Buffer
		if err := SidebarEntityList([]Entity{{ID: "e-1", Name: "Page", EntityTypeID: 7}}, nil, 1, 7, cc, nil).Render(context.Background(), &buf); err != nil {
			t.Fatalf("render failed: %v", err)
		}
		html := buf.String()
		if got := strings.Contains(html, `data-can-edit="true"`); got != tc.edit {
			t.Errorf("role %d: data-can-edit present = %v, want %v", tc.role, got, tc.edit)
		}
		if got := strings.Contains(html, `data-can-delete-folders="true"`); got != tc.delete {
			t.Errorf("role %d: data-can-delete-folders present = %v, want %v", tc.role, got, tc.delete)
		}
	}
}
