package entities

import "testing"

// castMemberIDs is a small helper for readable failure messages.
func castMemberIDs(ms []CastMember) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Entity.ID
	}
	return out
}

// TestAssembleCastParty pins the party assembly: the claimed set unioned with
// the viewer's own, deduped by ID and sorted viewer-first.
func TestAssembleCastParty(t *testing.T) {
	owner := func(s string) *string { return &s }
	ownerNames := map[string]string{"u1": "Alice", "u2": "Bob"}

	t.Run("unions claimed and own, dedups, viewer first", func(t *testing.T) {
		claimed := []Entity{
			{ID: "a", Name: "Aldric", OwnerUserID: owner("u1")},
			{ID: "b", Name: "Bryn", OwnerUserID: owner("u2")},
		}
		mine := []Entity{
			{ID: "b", Name: "Bryn", OwnerUserID: owner("u2")}, // duplicate of claimed
			{ID: "c", Name: "Cass", OwnerUserID: owner("u2")}, // own, not in claimed
		}

		party := assembleCastParty(claimed, mine, ownerNames, "u2")

		if len(party) != 3 {
			t.Fatalf("party = %v, want 3 deduped members", castMemberIDs(party))
		}
		// Viewer u2 owns b and c → both sort first; a (u1) last.
		if !party[0].IsViewer || !party[1].IsViewer || party[2].IsViewer {
			t.Errorf("viewer's characters should sort first; got %v", castMemberIDs(party))
		}
		for _, m := range party {
			if m.Entity.ID == "a" && m.OwnerName != "Alice" {
				t.Errorf("owner name for a = %q, want Alice", m.OwnerName)
			}
		}
	})

	t.Run("own private character still appears via mine", func(t *testing.T) {
		// "secret" is hidden from the visibility-filtered claimed list but the
		// owner must still see it — it arrives via the unfiltered mine list.
		claimed := []Entity{{ID: "a", Name: "Aldric", OwnerUserID: owner("u1")}}
		mine := []Entity{{ID: "secret", Name: "Hidden", OwnerUserID: owner("u2")}}

		party := assembleCastParty(claimed, mine, ownerNames, "u2")

		var found bool
		for _, m := range party {
			if m.Entity.ID == "secret" {
				found = true
				if !m.IsViewer {
					t.Errorf("own character should be marked IsViewer")
				}
			}
		}
		if !found {
			t.Errorf("own character missing from party: %v", castMemberIDs(party))
		}
	})
}
