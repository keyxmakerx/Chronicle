// claim_overview_test.go — PC-CLAIM-3. Pins the Player Character Claiming UI:
//
//	Part 1/4 — claimBanner: "Claimed by <player>" when owned; the actionable
//	           claim banner only when unclaimed + claimable + addon enabled.
//	Part 2   — claimRosterPanel: per-character owner + reassign/unclaim controls;
//	           ownerDisplayNames / resolveOwnerName / roster label helpers.
//	Part 3   — EntityTypeCard: the per-type "Players can claim" toggle appears
//	           and rides the save PUT only when the addon is enabled.
package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- pure helpers -----------------------------------------------------------

func TestOwnerDisplayNames(t *testing.T) {
	members := []campaigns.CampaignMember{
		{UserID: "u1", DisplayName: "Alice"},
		{UserID: "u2", DisplayName: "Bob"},
	}
	got := ownerDisplayNames(members)
	if got["u1"] != "Alice" || got["u2"] != "Bob" {
		t.Errorf("ownerDisplayNames = %v, want u1->Alice u2->Bob", got)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestResolveOwnerName(t *testing.T) {
	names := map[string]string{"u1": "Alice"}
	cases := []struct {
		name  string
		owner *string
		want  string
	}{
		{"unclaimed", nil, ""},
		{"known owner", strPtr("u1"), "Alice"},
		{"stale owner (no longer a member)", strPtr("u9"), ""},
	}
	for _, tc := range cases {
		if got := resolveOwnerName(tc.owner, names); got != tc.want {
			t.Errorf("%s: resolveOwnerName = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRosterOwnerLabel(t *testing.T) {
	roster := &ClaimRoster{OwnerNames: map[string]string{"u1": "Alice"}}
	cases := []struct {
		name string
		char Entity
		want string
	}{
		{"unclaimed", Entity{ID: "e1"}, "Unclaimed"},
		{"known", Entity{ID: "e2", OwnerUserID: strPtr("u1")}, "Alice"},
		{"stale", Entity{ID: "e3", OwnerUserID: strPtr("u9")}, "Unknown player"},
	}
	for _, tc := range cases {
		if got := rosterOwnerLabel(tc.char, roster); got != tc.want {
			t.Errorf("%s: rosterOwnerLabel = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestOwnerSelectValue(t *testing.T) {
	if got := ownerSelectValue(Entity{}); got != "" {
		t.Errorf("unclaimed select value = %q, want empty", got)
	}
	if got := ownerSelectValue(Entity{OwnerUserID: strPtr("u1")}); got != "u1" {
		t.Errorf("claimed select value = %q, want u1", got)
	}
}

// --- claimBanner (Parts 1 & 4) ---------------------------------------------

func renderClaimBanner(t *testing.T, entity *Entity, et *EntityType, claimingEnabled bool, ownerName string) string {
	t.Helper()
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "C"}, MemberRole: campaigns.RolePlayer}
	var buf bytes.Buffer
	if err := claimBanner(cc, entity, et, claimingEnabled, ownerName, "csrf-tok").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render claimBanner: %v", err)
	}
	return buf.String()
}

func TestClaimBanner_ClaimedShowsOwner(t *testing.T) {
	claimable := &EntityType{Slug: "character"}
	owned := &Entity{ID: "e1", Name: "Tyne", OwnerUserID: strPtr("u1")}

	// With the addon OFF the ownership indicator must still render — a claim is
	// always legible regardless of addon state.
	html := renderClaimBanner(t, owned, claimable, false, "Alice")
	for _, want := range []string{`data-claim-state="claimed"`, "Claimed by", "Alice"} {
		if !strings.Contains(html, want) {
			t.Errorf("claimed banner missing %q\n%s", want, html)
		}
	}
	if strings.Contains(html, `data-claim-state="unclaimed"`) {
		t.Error("claimed banner must not render the claim action")
	}
}

func TestClaimBanner_ClaimedUnknownOwnerFallback(t *testing.T) {
	owned := &Entity{ID: "e1", Name: "Tyne", OwnerUserID: strPtr("u1")}
	html := renderClaimBanner(t, owned, &EntityType{Slug: "character"}, true, "")
	if !strings.Contains(html, "a player") {
		t.Errorf("expected unknown-owner fallback 'a player'\n%s", html)
	}
}

func TestClaimBanner_UnclaimedGating(t *testing.T) {
	claimable := &EntityType{Slug: "character"}
	nonClaimable := &EntityType{Slug: "location"}
	unowned := &Entity{ID: "e1", Name: "Tyne"}

	cases := []struct {
		name         string
		et           *EntityType
		enabled      bool
		wantClaimBtn bool
	}{
		{"claimable + addon on → claim action", claimable, true, true},
		{"claimable + addon off → nothing", claimable, false, false},
		{"non-claimable + addon on → nothing", nonClaimable, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderClaimBanner(t, unowned, tc.et, tc.enabled, "")
			hasBtn := strings.Contains(html, "Claim character")
			if hasBtn != tc.wantClaimBtn {
				t.Errorf("claim button present=%v, want %v\n%s", hasBtn, tc.wantClaimBtn, html)
			}
			if tc.wantClaimBtn {
				// The form must POST to the claim endpoint.
				if !strings.Contains(html, "/campaigns/camp-1/entities/e1/claim") {
					t.Errorf("claim form missing POST target\n%s", html)
				}
			}
			// Never an owner indicator for an unclaimed entity.
			if strings.Contains(html, `data-claim-state="claimed"`) {
				t.Errorf("unclaimed entity rendered a claimed indicator\n%s", html)
			}
		})
	}
}

// --- claimRosterPanel (Part 2) ---------------------------------------------

func renderClaimRoster(t *testing.T, et *EntityType, roster *ClaimRoster) string {
	t.Helper()
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "C"}, MemberRole: campaigns.RoleScribe}
	var buf bytes.Buffer
	if err := claimRosterPanel(cc, et, roster).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render claimRosterPanel: %v", err)
	}
	return buf.String()
}

func TestClaimRosterPanel_RowsAndControls(t *testing.T) {
	et := &EntityType{ID: 1, Name: "Character", NamePlural: "Characters", Slug: "character"}
	roster := &ClaimRoster{
		Characters: []Entity{
			{ID: "e1", Name: "Tyne", OwnerUserID: strPtr("u1")},
			{ID: "e2", Name: "Bram"}, // unclaimed
		},
		Members: []campaigns.CampaignMember{
			{UserID: "u1", DisplayName: "Alice"},
			{UserID: "u2", DisplayName: "Bob"},
		},
		OwnerNames: map[string]string{"u1": "Alice", "u2": "Bob"},
	}
	html := renderClaimRoster(t, et, roster)

	for _, want := range []string{
		"Player Character Roster",
		"Tyne", "Bram", // both characters
		"Alice", "Bob", // member options
		"— Unclaimed —",                       // the unclaim option
		"Unclaim",                             // explicit unclaim button (Tyne is owned)
		"/campaigns/camp-1/entities/e1/owner", // reassign endpoint
		`value="u1"`,                          // Alice's option value
	} {
		if !strings.Contains(html, want) {
			t.Errorf("roster missing %q\n%s", want, html)
		}
	}

	// Tyne (owned) shows the owner name; Bram (unclaimed) is labeled Unclaimed.
	if !strings.Contains(html, "Unclaimed") {
		t.Errorf("expected an Unclaimed label for Bram\n%s", html)
	}
}

func TestClaimRosterPanel_Empty(t *testing.T) {
	et := &EntityType{ID: 1, Name: "Character", NamePlural: "Characters", Slug: "character"}
	roster := &ClaimRoster{
		Members:    []campaigns.CampaignMember{{UserID: "u1", DisplayName: "Alice"}},
		OwnerNames: map[string]string{"u1": "Alice"},
	}
	html := renderClaimRoster(t, et, roster)
	if !strings.Contains(html, "No Characters to assign yet") {
		t.Errorf("expected empty-state message\n%s", html)
	}
	if strings.Contains(html, "Unclaim") {
		t.Errorf("empty roster should have no row controls\n%s", html)
	}
}

// --- EntityTypeCard claim toggle (Part 3) ----------------------------------

func renderEntityTypeCard(t *testing.T, claimingEnabled bool) string {
	t.Helper()
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "C"}, MemberRole: campaigns.RoleOwner}
	et := &EntityType{ID: 5, Name: "Hero", NamePlural: "Heroes", Slug: "hero", Icon: "fa-user", Color: "#abcdef"}
	var buf bytes.Buffer
	if err := EntityTypeCard(cc, et, 0, "csrf", claimingEnabled).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render EntityTypeCard: %v", err)
	}
	return buf.String()
}

func TestEntityTypeCard_ClaimToggle(t *testing.T) {
	on := renderEntityTypeCard(t, true)
	if !strings.Contains(on, "Players can claim entities of this type") {
		t.Errorf("addon on: expected claim toggle label\n%s", on)
	}
	// The save PUT must carry the flag so the choice persists.
	if !strings.Contains(on, "claimable: etClaimable") {
		t.Errorf("addon on: save body must include the claimable flag\n%s", on)
	}

	off := renderEntityTypeCard(t, false)
	if strings.Contains(off, "Players can claim entities of this type") {
		t.Errorf("addon off: claim toggle must be hidden\n%s", off)
	}
	if strings.Contains(off, "claimable: etClaimable") {
		t.Errorf("addon off: save body must not touch the claimable flag\n%s", off)
	}
}

// The "Add Category" create form also gates the claimable checkbox on the addon.
func TestEntityTypeCreateForm_ClaimToggle(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "C"}, MemberRole: campaigns.RoleOwner}
	render := func(enabled bool) string {
		var buf bytes.Buffer
		if err := EntityTypeListContent(cc, nil, nil, "csrf", enabled).Render(context.Background(), &buf); err != nil {
			t.Fatalf("render list content: %v", err)
		}
		return buf.String()
	}
	if on := render(true); !strings.Contains(on, `name="claimable"`) {
		t.Errorf("addon on: create form must include the claimable checkbox\n%s", on)
	}
	if off := render(false); strings.Contains(off, `name="claimable"`) {
		t.Errorf("addon off: create form must not include the claimable checkbox\n%s", off)
	}
}
