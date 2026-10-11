package entities

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// The roster queries run against a real MariaDB: the playable column must
// round-trip through the shared SELECT, a claim must mark the page playable in
// the same statement, and the roster list must split exactly on
// playable-or-owned, honour visibility and the type filter, and treat LIKE
// wildcards in the search literally.
func TestEntityRepository_Roster_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	userID, campaignID := testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "roster-int-"+userID+"@example.test", "Roster Int Test", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Roster Int Test", "roster-int-"+campaignID[:8], userID)
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID)
	}()

	typeRepo := NewEntityTypeRepository(db)
	mkType := func(slug string) *EntityType {
		et := &EntityType{
			CampaignID: campaignID, Slug: slug, Name: slug, NamePlural: slug,
			Icon: "fa-user", Color: "#111111", Fields: []FieldDefinition{}, Layout: DefaultLayout(), Enabled: true,
		}
		mustCreate(t, typeRepo, ctx, et)
		return et
	}
	party, other := mkType("roster-party"), mkType("roster-other")

	repo := NewEntityRepository(db)
	n := 0
	mk := func(et *EntityType, name string, private bool) string {
		n++
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`, id, campaignID, et.ID, name, "r-"+strings.Repeat("x", n), private, userID)
		return id
	}
	npc := mk(party, "Npc Nora", false)
	marked := mk(party, "Marked Mara", false)
	claimed := mk(party, "Claimed Cleo", false)
	hidden := mk(party, "Hidden Hana", true)
	elsewhere := mk(other, "Other Opal", false)
	_ = mk(party, "100% Pure_Gold", false)

	if err := repo.SetPlayable(ctx, marked, true); err != nil {
		t.Fatalf("SetPlayable: %v", err)
	}
	if err := repo.SetPlayable(ctx, hidden, true); err != nil {
		t.Fatalf("SetPlayable: %v", err)
	}
	if err := repo.SetPlayable(ctx, elsewhere, true); err != nil {
		t.Fatalf("SetPlayable: %v", err)
	}

	names := func(es []Entity) string {
		var out []string
		for _, e := range es {
			out = append(out, e.Name)
		}
		return strings.Join(out, "|")
	}

	t.Run("playable round-trips through FindByID", func(t *testing.T) {
		e, err := repo.FindByID(ctx, marked)
		if err != nil || !e.Playable {
			t.Fatalf("marked: playable=%v err=%v", e != nil && e.Playable, err)
		}
		e, err = repo.FindByID(ctx, npc)
		if err != nil || e.Playable {
			t.Fatalf("npc: playable=%v err=%v", e != nil && e.Playable, err)
		}
	})

	t.Run("UpdateOwner marks playable and clearing keeps the mark", func(t *testing.T) {
		if err := repo.UpdateOwner(ctx, claimed, &userID); err != nil {
			t.Fatalf("UpdateOwner: %v", err)
		}
		e, _ := repo.FindByID(ctx, claimed)
		if !e.Playable || e.OwnerUserID == nil {
			t.Fatalf("after claim: playable=%v owner=%v", e.Playable, e.OwnerUserID)
		}
		if err := repo.UpdateOwner(ctx, claimed, nil); err != nil {
			t.Fatalf("UpdateOwner(nil): %v", err)
		}
		e, _ = repo.FindByID(ctx, claimed)
		if !e.Playable || e.OwnerUserID != nil {
			t.Fatalf("after unclaim: playable=%v owner=%v", e.Playable, e.OwnerUserID)
		}
		// Back to claimed-and-unmarked-by-nobody state for the list checks.
		if err := repo.UpdateOwner(ctx, claimed, &userID); err != nil {
			t.Fatalf("UpdateOwner: %v", err)
		}
		if err := repo.SetPlayable(ctx, claimed, false); err != nil {
			t.Fatalf("SetPlayable(false): %v", err)
		}
	})

	t.Run("ListOnRoster is playable or owned, by name, per viewer", func(t *testing.T) {
		got, err := repo.ListOnRoster(ctx, campaignID, []int{party.ID}, permissions.RoleOwner, userID)
		if err != nil {
			t.Fatalf("ListOnRoster: %v", err)
		}
		if want := "Claimed Cleo|Hidden Hana|Marked Mara"; names(got) != want {
			t.Errorf("owner sees %q, want %q (owned counts even when unmarked)", names(got), want)
		}
		got, _ = repo.ListOnRoster(ctx, campaignID, []int{party.ID}, permissions.RolePlayer, "someone-else")
		if want := "Claimed Cleo|Marked Mara"; names(got) != want {
			t.Errorf("player sees %q, want %q (private page hidden)", names(got), want)
		}
		got, _ = repo.ListOnRoster(ctx, campaignID, []int{party.ID, other.ID}, permissions.RoleOwner, userID)
		if !strings.Contains(names(got), "Other Opal") {
			t.Errorf("both types: %q lacks Other Opal", names(got))
		}
		if got, err := repo.ListOnRoster(ctx, campaignID, nil, permissions.RoleOwner, userID); got != nil || err != nil {
			t.Errorf("no types: got %v, %v; want nil, nil", got, err)
		}
	})

	t.Run("SearchRoster filters in SQL, lists free-to-pick first and escapes wildcards", func(t *testing.T) {
		search := func(role int, f RosterSearch) string {
			got, err := repo.SearchRoster(ctx, campaignID, []int{party.ID}, f, role, userID)
			if err != nil {
				t.Fatalf("SearchRoster(%+v): %v", f, err)
			}
			return names(got)
		}
		tests := []struct {
			name string
			role int
			f    RosterSearch
			want string
		}{
			{"free only, owner sees the private page", permissions.RoleOwner, RosterSearch{OnlyFree: true}, "Hidden Hana|Marked Mara"},
			{"free only, player does not", permissions.RolePlayer, RosterSearch{OnlyFree: true}, "Marked Mara"},
			{"a name search spans everything, free first", permissions.RoleOwner, RosterSearch{Query: "a"}, "Hidden Hana|Marked Mara|Claimed Cleo|Npc Nora"},
			{"adding to Free skips listed characters", permissions.RoleOwner, RosterSearch{Query: "a", SkipListed: true}, "Npc Nora"},
			{"giving skips what the target already plays", permissions.RoleOwner, RosterSearch{Query: "cleo", SkipOwnedBy: userID}, ""},
			{"giving keeps what someone else plays", permissions.RoleOwner, RosterSearch{Query: "cleo", SkipOwnedBy: "someone-else"}, "Claimed Cleo"},
			{"a bare % matches literally", permissions.RoleOwner, RosterSearch{Query: "%"}, "100% Pure_Gold"},
			{"a bare _ matches literally", permissions.RoleOwner, RosterSearch{Query: "_"}, "100% Pure_Gold"},
			{"other types stay out", permissions.RoleOwner, RosterSearch{Query: "opal"}, ""},
		}
		for _, tc := range tests {
			if got := search(tc.role, tc.f); got != tc.want {
				t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
			}
		}
		got, _ := repo.SearchRoster(ctx, campaignID, []int{party.ID}, RosterSearch{Query: "a", Limit: 1}, permissions.RoleOwner, userID)
		if len(got) != 1 {
			t.Errorf("limit 1 returned %d", len(got))
		}
		if got, err := repo.SearchRoster(ctx, campaignID, nil, RosterSearch{Query: "x"}, permissions.RoleOwner, userID); got != nil || err != nil {
			t.Errorf("no types: got %v, %v; want nil, nil", got, err)
		}
	})
}
