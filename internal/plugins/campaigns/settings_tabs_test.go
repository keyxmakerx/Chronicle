// settings_tabs_test.go pins the SettingsTab registry's role-filter +
// sort-order behavior. The refactor's load-bearing claim is "every
// existing tab renders identically post-refactor" — these tests
// enforce the per-tab MinRole + the canonical render order.
//
// If a future refactor flips a role gate (e.g. demoting AI Export to
// RoleScribe) without updating the dispatch + this test, the test
// fails with a clear pointer.
//
// Per cordinator/reports/chronicle/2026-05-26-c-ai-workspace-scoping.md
// §1.1 (registry recommendation) and §4 Phase 1 (the dispatch this
// implements).
package campaigns

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// builtInTabSpec is the expected shape of each built-in tab: ID +
// MinRole. Pinning these two together — and asserting the slice's
// length — catches every drift class we care about:
//   - a tab silently disappearing (length check)
//   - a tab silently appearing (length check)
//   - a tab's role gate silently changing (MinRole check)
//   - a tab's ID silently changing (ID check; query-string compat)
//
// SortOrder is checked indirectly via the slice's order matching this
// list.
type builtInTabSpec struct {
	ID      string
	MinRole Role
}

var builtInTabs = []builtInTabSpec{
	{"general", RolePlayer},
	{"features", RoleOwner},
	{"people", RolePlayer},
	{"integrations", RolePlayer},
	{"ai-export", RoleOwner},
	{"activity", RolePlayer},
}

func ctxWithRole(role Role) *CampaignContext {
	return &CampaignContext{
		Campaign:   &Campaign{ID: "camp-1", Name: "Test"},
		MemberRole: role,
	}
}

// TestSettingsTabs_OwnerSeesAllSixInOrder is the canonical pin —
// owner role + no plugin extras → built-ins in canonical order.
func TestSettingsTabs_OwnerSeesAllSixInOrder(t *testing.T) {
	h := &Handler{}
	cc := ctxWithRole(RoleOwner)

	got := h.visibleSettingsTabs(cc, nil, nil, "csrf", nil, nil, false)
	if len(got) != len(builtInTabs) {
		t.Fatalf("expected %d built-in tabs for owner, got %d: %+v",
			len(builtInTabs), len(got), tabIDs(got))
	}
	for i, want := range builtInTabs {
		if got[i].ID != want.ID {
			t.Errorf("tab[%d]: id=%q, want %q", i, got[i].ID, want.ID)
		}
		if got[i].MinRole != want.MinRole {
			t.Errorf("tab[%d] (%s): MinRole=%d, want %d", i, got[i].ID, got[i].MinRole, want.MinRole)
		}
	}
}

// TestSettingsTabs_PlayerDropsOwnerOnlyTabs pins the pre-refactor
// gating: Features + AI Export are owner-only; everything else is
// member-visible.
func TestSettingsTabs_PlayerDropsOwnerOnlyTabs(t *testing.T) {
	h := &Handler{}
	cc := ctxWithRole(RolePlayer)

	got := h.visibleSettingsTabs(cc, nil, nil, "csrf", nil, nil, false)
	gotIDs := tabIDs(got)

	wantIDs := []string{"general", "people", "integrations", "activity"}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("player should see %d tabs, got %d: %v", len(wantIDs), len(gotIDs), gotIDs)
	}
	for i, want := range wantIDs {
		if gotIDs[i] != want {
			t.Errorf("player tab[%d]=%q, want %q (full list: %v)", i, gotIDs[i], want, gotIDs)
		}
	}
	for _, id := range gotIDs {
		if id == "features" || id == "ai-export" {
			t.Errorf("player leaked owner-only tab %q", id)
		}
	}
}

// TestSettingsTabs_ScribeMatchesOwnerMinusOwnerGates — Scribe role
// sits between Player and Owner; should see the same four
// player-visible tabs (no owner-only gate clears for Scribe in the
// pre-refactor templ).
func TestSettingsTabs_ScribeMatchesOwnerMinusOwnerGates(t *testing.T) {
	h := &Handler{}
	cc := ctxWithRole(RoleScribe)

	got := h.visibleSettingsTabs(cc, nil, nil, "csrf", nil, nil, false)
	for _, tab := range got {
		if tab.ID == "features" || tab.ID == "ai-export" {
			t.Errorf("scribe leaked owner-only tab %q (pre-refactor templ gated these on >= RoleOwner)", tab.ID)
		}
	}
	wantCount := 4 // general, people, integrations, activity
	if len(got) != wantCount {
		t.Errorf("scribe should see %d tabs, got %d: %v", wantCount, len(got), tabIDs(got))
	}
}

// TestRegisterSettingsTab_MergesAndSorts confirms plugin-contributed
// tabs land in the right position by SortOrder. Phase 2's AI Workspace
// plugin registers at SortOrder 55 — between AI Export (50) and
// Activity (60) — so this test pins that specific landing.
func TestRegisterSettingsTab_MergesAndSorts(t *testing.T) {
	h := &Handler{}
	h.RegisterSettingsTab(SettingsTab{
		ID:        "ai-workspace",
		Label:     "AI Workspace",
		Icon:      "fa-solid fa-brain",
		MinRole:   RoleOwner,
		SortOrder: 55,
	})

	cc := ctxWithRole(RoleOwner)
	got := h.visibleSettingsTabs(cc, nil, nil, "csrf", nil, nil, false)
	gotIDs := tabIDs(got)
	want := []string{"general", "features", "people", "integrations", "ai-export", "ai-workspace", "activity"}
	if len(gotIDs) != len(want) {
		t.Fatalf("merged tabs len=%d, want %d: got %v want %v", len(gotIDs), len(want), gotIDs, want)
	}
	for i, w := range want {
		if gotIDs[i] != w {
			t.Errorf("merged tab[%d]=%q, want %q (full: %v)", i, gotIDs[i], w, gotIDs)
		}
	}
}

// TestRegisterSettingsTab_StableSortOnTie confirms plugins that
// register multiple tabs at the same SortOrder preserve their
// registration order (matters for plugins contributing related tabs
// like "AI Workspace > Prompt" + "AI Workspace > Import").
func TestRegisterSettingsTab_StableSortOnTie(t *testing.T) {
	h := &Handler{}
	h.RegisterSettingsTab(SettingsTab{ID: "a", MinRole: RolePlayer, SortOrder: 100})
	h.RegisterSettingsTab(SettingsTab{ID: "b", MinRole: RolePlayer, SortOrder: 100})
	h.RegisterSettingsTab(SettingsTab{ID: "c", MinRole: RolePlayer, SortOrder: 100})

	cc := ctxWithRole(RolePlayer)
	got := h.visibleSettingsTabs(cc, nil, nil, "csrf", nil, nil, false)
	// Filter to just the plugin extras (built-ins use 10..60).
	var extra []string
	for _, t := range got {
		if t.SortOrder >= 100 {
			extra = append(extra, t.ID)
		}
	}
	want := []string{"a", "b", "c"}
	if len(extra) != len(want) {
		t.Fatalf("extras: got %v, want %v", extra, want)
	}
	for i, w := range want {
		if extra[i] != w {
			t.Errorf("extras[%d]=%q, want %q (full: %v)", i, extra[i], w, extra)
		}
	}
}

// TestSettingsTabs_RoleConstantsMatchPermissionsPackage guards
// against a future refactor that decouples campaigns.Role from the
// internal/permissions constants. The scoping report's recommendation
// + this whole refactor depend on the role hierarchy staying
// 1=Player, 2=Scribe, 3=Owner — same as `permissions.Role*`.
func TestSettingsTabs_RoleConstantsMatchPermissionsPackage(t *testing.T) {
	cases := []struct {
		got, want int
		label     string
	}{
		{int(RolePlayer), permissions.RolePlayer, "Player"},
		{int(RoleScribe), permissions.RoleScribe, "Scribe"},
		{int(RoleOwner), permissions.RoleOwner, "Owner"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: campaigns.Role=%d, permissions.Role=%d — must match",
				c.label, c.got, c.want)
		}
	}
}

// tabIDs is a small helper that extracts the IDs from a tab slice
// for cleaner failure messages.
func tabIDs(tabs []SettingsTab) []string {
	out := make([]string, len(tabs))
	for i, t := range tabs {
		out[i] = t.ID
	}
	return out
}
