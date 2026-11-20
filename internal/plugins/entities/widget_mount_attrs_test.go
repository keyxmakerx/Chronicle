package entities

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// A system widget reads what the viewer may do and who has claimed the page
// from the mount's attributes, so each one has to match the server's own rule.
func TestMakeWidgetMountRenderer_PageAttributes(t *testing.T) {
	renderer := MakeWidgetMountRenderer("w")
	robin := "u-robin"
	claimed := &Entity{ID: "e1", CampaignID: "c1", OwnerUserID: &robin}
	free := &Entity{ID: "e1", CampaignID: "c1"}

	cases := []struct {
		name    string
		entity  *Entity
		role    campaigns.Role
		userID  string
		owner   string
		panel   bool
		wantAll []string
	}{
		{"claimant", claimed, campaigns.RolePlayer, robin, "Robin", true, []string{
			`data-can-edit-identity="true"`, `data-can-change-image="true"`, `data-armory-items="true"`,
			`data-claimed="true"`, `data-claimed-by-me="true"`, `data-claimed-name="Robin"`, `data-foundry-linked="false"`}},
		{"another player", claimed, campaigns.RolePlayer, "u-other", "Robin", false, []string{
			`data-can-edit-identity="false"`, `data-can-change-image="false"`, `data-armory-items="false"`,
			`data-claimed="true"`, `data-claimed-by-me="false"`, `data-claimed-name="Robin"`}},
		{"scribe on a claimed page", claimed, campaigns.RoleScribe, "u-scribe", "Robin", true, []string{
			`data-can-edit-identity="true"`, `data-can-change-image="true"`, `data-claimed-by-me="false"`}},
		{"GM on a free page", free, campaigns.RoleOwner, "u-gm", "", false, []string{
			`data-can-edit-identity="true"`, `data-can-change-image="true"`,
			`data-claimed="false"`, `data-claimed-by-me="false"`, `data-claimed-name=""`}},
		{"player on a free page", free, campaigns.RolePlayer, "u-other", "", false, []string{
			`data-can-edit-identity="false"`, `data-claimed="false"`}},
		{"claimed, name not resolved", claimed, campaigns.RolePlayer, "u-other", "", false, []string{
			`data-claimed="true"`, `data-claimed-name="a player"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.panel {
				ctx = withAutoPagePanel(ctx)
			}
			buf := &bytesBufferLike{}
			err := renderer(EntityShowRenderContext{
				CC:     &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: tc.role},
				Entity: tc.entity, UserID: tc.userID, OwnerName: tc.owner,
			}).Render(ctx, buf)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tc.wantAll {
				if !strings.Contains(buf.String(), w) {
					t.Errorf("missing %s in %s", w, buf.String())
				}
			}
		})
	}
}

func TestMakeWidgetMountRenderer_EscapesClaimedName(t *testing.T) {
	robin := "u"
	buf := &bytesBufferLike{}
	err := MakeWidgetMountRenderer("w")(EntityShowRenderContext{
		Entity: &Entity{ID: "e1", OwnerUserID: &robin}, OwnerName: `"><script>x</script>`,
	}).Render(context.Background(), buf)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "<script>") {
		t.Errorf("claimed name not escaped: %s", buf.String())
	}
}

// A visitor can't fetch the entity API, so the mount carries what they may
// read of the page; GM-only values stay out, and a member still fetches.
func TestMakeWidgetMountRenderer_VisitorSeed(t *testing.T) {
	renderer := MakeWidgetMountRenderer("w")
	e := &Entity{ID: "e1", CampaignID: "c1", Name: "Vex", FieldsData: map[string]any{"might": "2", "secret": "a spy"}}
	et := &EntityType{Fields: []FieldDefinition{{Key: "might"}, {Key: "secret", GMOnly: true}}}
	cases := []struct {
		name     string
		member   bool
		wantSeed bool
	}{
		{"visitor", false, true},
		{"member", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytesBufferLike{}
			cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleNone, IsMember: tc.member}
			if tc.member {
				cc.MemberRole = campaigns.RolePlayer
			}
			if err := renderer(EntityShowRenderContext{CC: cc, Entity: e, EntityType: et}).Render(context.Background(), buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if got := strings.Contains(out, "data-fields-data="); got != tc.wantSeed {
				t.Fatalf("seed = %v, want %v: %s", got, tc.wantSeed, out)
			}
			if tc.wantSeed && (!strings.Contains(out, "&#34;might&#34;:&#34;2&#34;") || strings.Contains(out, "a spy")) {
				t.Fatalf("seed should hold might and not the GM-only value: %s", out)
			}
		})
	}
}
