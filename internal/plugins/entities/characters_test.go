package entities

import "testing"

// memberIDs / npcIDs are small helpers for readable failure messages.
func castMemberIDs(ms []CastMember) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Entity.ID
	}
	return out
}

// TestAssembleCastView pins the Characters page assembly rules: the party is the
// claimed set unioned with the viewer's own (deduped, viewer-first) and the
// active NPCs are the cast-tagged entities that are not claimed.
func TestAssembleCastView(t *testing.T) {
	owner := func(s string) *string { return &s }
	ownerNames := map[string]string{"u1": "Alice", "u2": "Bob"}

	t.Run("party unions claimed and own, dedups, viewer first", func(t *testing.T) {
		claimed := []Entity{
			{ID: "a", Name: "Aldric", OwnerUserID: owner("u1")},
			{ID: "b", Name: "Bryn", OwnerUserID: owner("u2")},
		}
		mine := []Entity{
			{ID: "b", Name: "Bryn", OwnerUserID: owner("u2")}, // duplicate of claimed
			{ID: "c", Name: "Cass", OwnerUserID: owner("u2")}, // own, not in claimed
		}

		view := assembleCastView(claimed, mine, nil, ownerNames, "u2", false)

		if len(view.Party) != 3 {
			t.Fatalf("party = %v, want 3 deduped members", castMemberIDs(view.Party))
		}
		// Viewer u2 owns b and c → both sort first; a (u1) last.
		if !view.Party[0].IsViewer || !view.Party[1].IsViewer || view.Party[2].IsViewer {
			t.Errorf("viewer's characters should sort first; got %v", castMemberIDs(view.Party))
		}
		for _, m := range view.Party {
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

		view := assembleCastView(claimed, mine, nil, ownerNames, "u2", false)

		var found bool
		for _, m := range view.Party {
			if m.Entity.ID == "secret" {
				found = true
				if !m.IsViewer {
					t.Errorf("own character should be marked IsViewer")
				}
			}
		}
		if !found {
			t.Errorf("own character missing from party: %v", castMemberIDs(view.Party))
		}
	})

	t.Run("active NPCs exclude claimed characters and pass CanCurate", func(t *testing.T) {
		tagged := []Entity{
			{ID: "n1", Name: "Innkeeper"},                      // unowned NPC
			{ID: "p1", Name: "Hero", OwnerUserID: owner("u1")}, // a PC that is also cast-tagged
		}

		view := assembleCastView(nil, nil, tagged, ownerNames, "u2", true)

		if len(view.ActiveNPCs) != 1 || view.ActiveNPCs[0].Entity.ID != "n1" {
			t.Fatalf("active NPCs = %v, want only [n1]", castMemberIDs(view.ActiveNPCs))
		}
		if !view.CanCurate {
			t.Errorf("CanCurate should pass through")
		}
	})
}
