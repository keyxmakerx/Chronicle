package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

func renderHeader(t *testing.T, role campaigns.Role, userID string, entity *Entity, claimingEnabled bool) string {
	t.Helper()
	ctx := layouts.SetUserID(context.Background(), userID)
	et := &EntityType{ID: 1, Slug: "character", PresetCategory: strPtr("character")}
	var buf bytes.Buffer
	if err := systemPageHeader(foldTestCC(role), entity, et, claimingEnabled, "Robin", "tok", userID).Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestHeaderClaimState(t *testing.T) {
	owner := "u-robin"
	claimed := &Entity{OwnerUserID: &owner}
	free := &Entity{}
	cases := []struct {
		name      string
		entity    *Entity
		userID    string
		scribe    bool
		enabled   bool
		claimable bool
		want      claimPillState
	}{
		{"claimant sees Yours", claimed, owner, false, true, true, claimMine},
		{"claimant sees Yours with the addon off", claimed, owner, false, false, true, claimMine},
		{"another player sees who claimed it", claimed, "u-other", false, true, true, claimOther},
		{"the GM sees who claimed it", claimed, "u-gm", true, true, true, claimOther},
		{"GM sees not claimed yet", free, "u-gm", true, true, true, claimNotYet},
		{"player gets the Claim button", free, "u-other", false, true, true, claimAvailable},
		{"addon off shows nothing", free, "u-other", false, false, true, claimNone},
		{"unclaimable type shows nothing", free, "u-other", false, true, false, claimNone},
	}
	for _, tc := range cases {
		if got := headerClaimState(tc.entity, tc.userID, tc.scribe, tc.enabled, tc.claimable); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestSystemPageHeader(t *testing.T) {
	owner := "u-robin"
	claimed := &Entity{ID: "e1", CampaignID: "c1", Name: "Bren", OwnerUserID: &owner}
	free := &Entity{ID: "e1", CampaignID: "c1", Name: "Bren"}

	cases := []struct {
		name   string
		role   campaigns.Role
		userID string
		entity *Entity
		want   []string
		not    []string
	}{
		{"GM: every action, claim pill, confirm without a native dialog",
			campaigns.RoleOwner, "u-gm", claimed,
			[]string{`data-page-h1`, `data-favorite-toggle="e1"`, "Claimed by Robin", "Edit name", "History", "Clone", "Delete", `data-del-ask`, `page-history-slot`, `id="page-name-fold"`},
			[]string{"hx-confirm", "confirm(", "alert("}},
		{"scribe: Clone but no Delete",
			campaigns.RoleScribe, "u-scribe", claimed,
			[]string{"Clone", "History", "Edit name"},
			[]string{"data-del-ask", "hx-delete"}},
		{"claimed player: Yours, Edit name only, phone-only menu",
			campaigns.RolePlayer, owner, claimed,
			[]string{"Yours", "Edit name", "ph-phone-only"},
			[]string{"History", "Clone", "Delete", "page-history-slot", `data-parent-q`}},
		{"other player: no actions at all",
			campaigns.RolePlayer, "u-other", claimed,
			[]string{"Claimed by Robin", `data-page-h1`},
			[]string{"Edit name", "History", "Clone", "Delete", "data-more", "page-name-fold"}},
		{"player sees the Claim button when free",
			campaigns.RolePlayer, "u-other", free,
			[]string{"Claim character", `data-claim-warn`, "/campaigns/c1/entities/e1/claim"},
			[]string{"Edit name", "Delete"}},
		{"GM sees Not claimed yet",
			campaigns.RoleOwner, "u-gm", free,
			[]string{"Not claimed yet"},
			[]string{"Claim character"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderHeader(t, tc.role, tc.userID, tc.entity, true)
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(html, n) {
					t.Errorf("must not contain %q", n)
				}
			}
		})
	}
}

// Delete keeps today's role rule: only the campaign owner role gets it.
func TestSystemPageHeader_DeleteIsOwnerOnly(t *testing.T) {
	ent := &Entity{ID: "e1", CampaignID: "c1", Name: "Bren"}
	for _, tc := range []struct {
		role campaigns.Role
		want bool
	}{{campaigns.RolePlayer, false}, {campaigns.RoleScribe, false}, {campaigns.RoleOwner, true}} {
		got := strings.Contains(renderHeader(t, tc.role, "u", ent, true), "data-del-ask")
		if got != tc.want {
			t.Errorf("role %v: delete confirm = %v, want %v", tc.role, got, tc.want)
		}
	}
}

// The picture upload the widget's Change button reaches exists only for those
// who may replace the picture (the image route is Scribe-only).
func TestSystemImageMount_ScribeOnly(t *testing.T) {
	ent := &Entity{ID: "e1", CampaignID: "c1"}
	for _, tc := range []struct {
		role campaigns.Role
		want bool
	}{{campaigns.RolePlayer, false}, {campaigns.RoleScribe, true}, {campaigns.RoleOwner, true}} {
		var buf bytes.Buffer
		if err := systemImageMount(foldTestCC(tc.role), ent, "tok").Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		got := strings.Contains(buf.String(), `data-image-upload-for="e1"`) &&
			strings.Contains(buf.String(), `data-endpoint="/campaigns/c1/entities/e1/image"`)
		if got != tc.want {
			t.Errorf("role %v: mount = %v, want %v", tc.role, got, tc.want)
		}
	}
}

// The panel under a system widget carries a tab for each piece this viewer
// can see, in order, with every piece's own block in its pane.
func TestSystemPageTabs_TabsPerViewer(t *testing.T) {
	prev := GetGlobalBlockRegistry()
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	SetGlobalBlockRegistry(reg)
	defer SetGlobalBlockRegistry(prev)

	ent := &Entity{ID: "e1", CampaignID: "c1", Name: "Bren"}
	et := &EntityType{ID: 1, Slug: "character"}
	withPanel := func(ctx context.Context) context.Context {
		return withPagePanelURL(withAutoPagePanel(ctx), "/campaigns/c1/armory/characters/e1/panel")
	}
	cases := []struct {
		name  string
		role  campaigns.Role
		user  string
		panel bool
		want  []string
	}{
		{"player with items", campaigns.RolePlayer, "u1", true, []string{"notes", "items", "files", "posts", "links"}},
		{"player, no armory panel", campaigns.RolePlayer, "u1", false, []string{"notes", "files", "posts", "links"}},
		{"public visitor", campaigns.RoleNone, "", false, []string{"posts", "links"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := layouts.SetUserID(withPageChildren(context.Background(), nil), tc.user)
			if tc.panel {
				ctx = withPanel(ctx)
			}
			var buf bytes.Buffer
			if err := systemPageTabs(foldTestCC(tc.role), ent, et, "tok").Render(ctx, &buf); err != nil {
				t.Fatal(err)
			}
			html := buf.String()
			var got []string
			for _, part := range strings.Split(html, `data-pt-tab="`)[1:] {
				got = append(got, part[:strings.Index(part, `"`)])
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("tabs = %v, want %v", got, tc.want)
			}
			if !strings.Contains(html, `data-widget="page-tabs"`) {
				t.Error("panel is not the page-tabs widget")
			}
			// Only the first tab's pane is open on arrival.
			if n := strings.Count(html, `aria-selected="true"`); n != 1 {
				t.Errorf("%d tabs selected, want 1", n)
			}
			for _, w := range []string{`data-widget="relations"`, `data-widget="tag-picker"`, `data-widget="entity-posts"`, "/backlinks"} {
				if !strings.Contains(html, w) {
					t.Errorf("missing %s", w)
				}
			}
			if got := strings.Contains(html, `data-widget="entity-notes"`); got != (tc.user != "") {
				t.Errorf("notes mount = %v, want %v", got, tc.user != "")
			}
			if got := strings.Contains(html, "/armory/characters/e1/panel"); got != tc.panel {
				t.Errorf("items mount = %v, want %v", got, tc.panel)
			}
		})
	}
}
