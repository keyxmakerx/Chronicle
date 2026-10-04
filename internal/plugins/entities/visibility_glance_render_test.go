// visibility_glance_render_test.go renders the REAL call sites (the page corner,
// EntityCard, blockDetails, EntityTableRow, entityTreeLevel, blockChildren,
// entityBlock/RenderBlock), never a hand-rolled stand-in for them (ADR-057).
package entities

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// renderComponent renders a templ.Component to a string, failing the test on
// a render error.
func renderComponent(t *testing.T, c templ.Component) string {
	t.Helper()
	var sb strings.Builder
	if err := c.Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

// glanceViewers is the set of viewer shapes exercised against every
// visibilityGlance call site. Player and anonymous get no glance element at
// all (a badge on something a Player *can* see would itself leak that others
// can't, ADR-055 rule 3); co-DM and Owner get one — the gate is
// VisibilityRole(), not raw MemberRole, so a DM-granted co-DM is treated like
// an Owner (ADR-057).
func glanceViewers() map[string]*campaigns.CampaignContext {
	camp := &campaigns.Campaign{ID: "c1"}
	return map[string]*campaigns.CampaignContext{
		"player":    {Campaign: camp, MemberRole: campaigns.RolePlayer, IsMember: true},
		"anonymous": {Campaign: camp, MemberRole: campaigns.RoleNone, IsAnonymous: true},
		"co-dm":     {Campaign: camp, MemberRole: campaigns.RolePlayer, IsDmGranted: true, IsMember: true},
		"owner":     {Campaign: camp, MemberRole: campaigns.RoleOwner, IsMember: true},
	}
}

// TestVisibilityGlance_GateAcrossCallSites pins the visibility-glance gate
// across every render call site, not just the show page header: Player and
// anonymous viewers must get no glance element, and co-DM/Owner must, on
// the page corner, EntityCard, blockDetails, EntityTableRow, entityTreeLevel and
// blockChildren alike (ADR-057).
func TestVisibilityGlance_GateAcrossCallSites(t *testing.T) {
	entity := &Entity{ID: "e1", CampaignID: "c1", EntityTypeID: 9, Name: "Waterdeep", Visibility: VisibilityDefault, IsPrivate: true}
	child := *entity
	node := EntityTreeNode{Entity: *entity}

	sites := map[string]func(cc *campaigns.CampaignContext) string{
		"page corner (effectiveVisibilityBadge)": func(cc *campaigns.CampaignContext) string {
			return renderComponent(t, effectiveVisibilityBadge(cc, entity, nil))
		},
		"card (EntityCard)": func(cc *campaigns.CampaignContext) string {
			return renderComponent(t, EntityCard(entity, cc))
		},
		"details (blockDetails)": func(cc *campaigns.CampaignContext) string {
			return renderComponent(t, blockDetails(cc, entity))
		},
		"table row (EntityTableRow)": func(cc *campaigns.CampaignContext) string {
			return renderComponent(t, EntityTableRow(entity, cc))
		},
		"tree node (entityTreeLevel)": func(cc *campaigns.CampaignContext) string {
			return renderComponent(t, entityTreeLevel(cc, []EntityTreeNode{node}, 0))
		},
		"child list (blockChildren)": func(cc *campaigns.CampaignContext) string {
			return renderComponent(t, blockChildren(cc, entity, []Entity{child}))
		},
	}

	viewers := glanceViewers()
	for siteName, render := range sites {
		t.Run(siteName, func(t *testing.T) {
			for _, hidden := range []string{"player", "anonymous"} {
				html := render(viewers[hidden])
				if strings.Contains(html, "data-visibility-badge") {
					t.Errorf("%s viewer must get no glance element at all, found one:\n%s", hidden, html)
				}
			}
			for _, visible := range []string{"co-dm", "owner"} {
				html := render(viewers[visible])
				if !strings.Contains(html, "data-visibility-badge") {
					t.Errorf("%s viewer must see the glance, got none:\n%s", visible, html)
				}
			}
		})
	}
}

// TestVisibilityGlance_ThreeStates pins the glyph-per-state mapping through a
// real call site (the card), not the component in isolation: globe for
// everyone, lock for dm_only, shield for custom, and never more than one of
// the three on a given render.
func TestVisibilityGlance_ThreeStates(t *testing.T) {
	owner := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner}
	glyphs := []string{"fa-globe", "fa-lock", "fa-shield-halved"}
	cases := []struct {
		name   string
		entity *Entity
		want   string
	}{
		{"everyone", &Entity{ID: "e1", Visibility: VisibilityDefault, IsPrivate: false}, "fa-globe"},
		{"dm_only", &Entity{ID: "e2", Visibility: VisibilityDefault, IsPrivate: true}, "fa-lock"},
		{"custom", &Entity{ID: "e3", Visibility: VisibilityCustom}, "fa-shield-halved"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderComponent(t, EntityCard(tc.entity, owner))
			if !strings.Contains(html, tc.want) {
				t.Errorf("expected %q in rendered card, got:\n%s", tc.want, html)
			}
			for _, other := range glyphs {
				if other != tc.want && strings.Contains(html, other) {
					t.Errorf("did not expect %q alongside %q, got:\n%s", other, tc.want, html)
				}
			}
		})
	}
}

// TestVisibilityGlance_TagWidened pins the tag-widening safety contract
// through the header call site (the one that threads EffectiveVisibility): a
// tag grant exposing an otherwise dm_only entity must show the amber corner
// dot and name the tag+subject, never silently upgrade the badge or drop the
// exposure.
func TestVisibilityGlance_TagWidened(t *testing.T) {
	owner := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner}
	entity := &Entity{ID: "e1", Visibility: VisibilityDefault, IsPrivate: true}
	ev := &EffectiveVisibility{
		BaseState:     VisStateDMOnly,
		WidenedByTags: true,
		TagGrants:     []EntityTagGrantInfo{{TagSlug: "revealed-act-1", SubjectLabel: "Players"}},
	}
	ctx := WithEffectiveVisibility(context.Background(), ev)
	var sb strings.Builder
	if err := effectiveVisibilityBadge(owner, entity, GetEffectiveVisibility(ctx)).Render(ctx, &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	for _, want := range []string{"fa-lock", "bg-amber-400", `data-tag-widened="true"`, "Players via", "revealed-act-1"} {
		if !strings.Contains(html, want) {
			t.Errorf("tag-widened header missing %q\ngot: %s", want, html)
		}
	}
}

// TestStoredRowPermRendersWithoutPermissionsBlock pins ADR-057 decision 5: a
// stored layout_json can still carry a block of Type "permissions" (migrations
// are append-only, so nothing rewrites it), and RenderBlock must drop that
// unregistered type silently rather than erroring, against the real
// production registry (RegisterCoreBlocks).
func TestStoredRowPermRendersWithoutPermissionsBlock(t *testing.T) {
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	SetGlobalBlockRegistry(reg)
	t.Cleanup(func() { SetGlobalBlockRegistry(nil) })

	if reg.IsValid("permissions") {
		t.Fatalf(`the "permissions" block type must no longer be registered (ADR-057 decision 5)`)
	}

	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	entity := &Entity{ID: "e1", CampaignID: "camp-1"}
	entityType := &EntityType{ID: 1}
	legacyLayout := EntityTypeLayout{Rows: []TemplateRow{
		{ID: "row-1", Columns: []TemplateColumn{{ID: "c1", Width: 8, Blocks: []TemplateBlock{{ID: "b-entry", Type: "entry"}}}}},
		{ID: "row-perm", Columns: []TemplateColumn{{ID: "col-perm", Width: 12, Blocks: []TemplateBlock{{ID: "blk-perm", Type: "permissions"}}}}},
	}}

	var sb strings.Builder
	for _, row := range legacyLayout.Rows {
		for _, col := range row.Columns {
			for _, block := range col.Blocks {
				if err := entityBlock(block, cc, entity, entityType, false, "csrf").Render(context.Background(), &sb); err != nil {
					t.Fatalf("rendering a stored row-perm layout must not error, got: %v", err)
				}
			}
		}
	}
	html := sb.String()
	if strings.Contains(html, `data-widget="permissions"`) {
		t.Errorf("permissions widget mount must not render for a stored row-perm layout, got:\n%s", html)
	}
	if !strings.Contains(html, `data-widget="editor"`) {
		t.Errorf("the sibling entry block must still render, got:\n%s", html)
	}
}

// TestPageCorner_OnlyOwnerGetsEditor pins who can open the permissions
// editor from the page corner: the Owner gets the corner widget mount, while
// a member the owner has given DM access sees the control but no editor (the
// permissions routes are Owner-only). The title block no longer carries a
// second visibility icon.
func TestPageCorner_OnlyOwnerGetsEditor(t *testing.T) {
	entity := &Entity{ID: "e1", CampaignID: "c1", Name: "The Gilded Anvil", Visibility: VisibilityDefault}
	viewers := glanceViewers()

	owner := renderComponent(t, effectiveVisibilityBadge(viewers["owner"], entity, nil))
	for _, want := range []string{`data-widget="permissions"`, `data-layout="corner"`, `/campaigns/c1/entities/e1/permissions`, "data-perm-corner-trigger"} {
		if !strings.Contains(owner, want) {
			t.Errorf("owner corner missing %q\ngot: %s", want, owner)
		}
	}

	coDM := renderComponent(t, effectiveVisibilityBadge(viewers["co-dm"], entity, nil))
	if strings.Contains(coDM, "data-widget") {
		t.Errorf("a member with DM access must not get the editor mount:\n%s", coDM)
	}

	title := renderComponent(t, blockTitle(viewers["owner"], entity, "csrf"))
	if strings.Contains(title, "data-visibility-badge") {
		t.Errorf("title block must not carry a second visibility icon:\n%s", title)
	}
	h1, aka := strings.Index(title, "<h1"), strings.Index(title, `data-widget="aliases"`)
	if h1 < 0 || aka < h1 {
		t.Errorf("aliases must render after the page name")
	}
}
