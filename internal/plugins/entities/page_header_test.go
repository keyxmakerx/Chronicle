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

func TestSystemPageBelow_RendersTheFourBlocks(t *testing.T) {
	prev := GetGlobalBlockRegistry()
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	SetGlobalBlockRegistry(reg)
	defer SetGlobalBlockRegistry(prev)

	ent := &Entity{ID: "e1", CampaignID: "c1", Name: "Bren"}
	ctx := layouts.SetUserID(withPageChildren(context.Background(), nil), "u1")
	var buf bytes.Buffer
	et := &EntityType{ID: 1, Slug: "character"}
	if err := systemPageBelow(foldTestCC(campaigns.RolePlayer), ent, et, "tok").Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, w := range []string{`data-widget="relations"`, `data-widget="tag-picker"`} {
		if !strings.Contains(html, w) {
			t.Errorf("missing %s", w)
		}
	}
}
