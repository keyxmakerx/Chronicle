// event_kind_repository_test.go exercises EventKindRepository against a real
// MariaDB: table-driven CRUD, tenant isolation (a campaign can never read or
// mutate another campaign's kind, even by guessing its numeric id), and the
// cascade from a deleted campaign. Skipped under `-short`.
package calendar

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestEventKindRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() }) // after fixture cleanups (LIFO), not before

	ctx := context.Background()
	repo := NewEventKindRepository(db)

	fixA := newTestCampaign(t, db, "kind-a")
	fixB := newTestCampaign(t, db, "kind-b")

	t.Run("table-driven CRUD", func(t *testing.T) {
		tests := []struct {
			name  string
			input EventKindInput
		}{
			{"ahead-default kind", EventKindInput{Slug: "holiday", Name: "Holiday", Icon: "⭐", Color: "#84cc16", DefaultAnnounced: AnnouncedAhead}},
			{"on_day-default kind", EventKindInput{Slug: "battle", Name: "Battle", Icon: "⚔", Color: "#ef4444", DefaultAnnounced: AnnouncedOnDay}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				created, err := repo.Create(ctx, fixA.CampaignID, tt.input)
				if err != nil {
					t.Fatalf("Create: %v", err)
				}
				if created.Slug != tt.input.Slug || created.DefaultAnnounced != tt.input.DefaultAnnounced {
					t.Fatalf("Create returned %+v, want slug=%q announced=%q", created, tt.input.Slug, tt.input.DefaultAnnounced)
				}

				got, err := repo.GetByID(ctx, created.ID, fixA.CampaignID)
				if err != nil || got == nil {
					t.Fatalf("GetByID = %+v, %v", got, err)
				}
				if got.CampaignID != fixA.CampaignID {
					t.Errorf("CampaignID = %q, want %q", got.CampaignID, fixA.CampaignID)
				}

				updated := tt.input
				updated.Name = tt.input.Name + " (updated)"
				if err := repo.Update(ctx, created.ID, fixA.CampaignID, updated); err != nil {
					t.Fatalf("Update: %v", err)
				}
				got, _ = repo.GetByID(ctx, created.ID, fixA.CampaignID)
				if got.Name != updated.Name {
					t.Errorf("Update did not persist: got name %q", got.Name)
				}

				if err := repo.Delete(ctx, created.ID, fixA.CampaignID); err != nil {
					t.Fatalf("Delete: %v", err)
				}
				got, err = repo.GetByID(ctx, created.ID, fixA.CampaignID)
				if err != nil || got != nil {
					t.Errorf("GetByID after Delete = %+v, %v; want (nil, nil)", got, err)
				}
			})
		}
	})

	t.Run("List returns only this campaign's kinds, ordered by sort_order", func(t *testing.T) {
		k1, err := repo.Create(ctx, fixA.CampaignID, EventKindInput{Slug: "quest", Name: "Quest", SortOrder: 1, DefaultAnnounced: AnnouncedOnDay})
		if err != nil {
			t.Fatalf("Create k1: %v", err)
		}
		k2, err := repo.Create(ctx, fixA.CampaignID, EventKindInput{Slug: "festival", Name: "Festival", SortOrder: 0, DefaultAnnounced: AnnouncedAhead})
		if err != nil {
			t.Fatalf("Create k2: %v", err)
		}
		// The same slug is allowed in a different campaign: the unique key is
		// (campaign_id, slug), not slug alone.
		if _, err := repo.Create(ctx, fixB.CampaignID, EventKindInput{Slug: "quest", Name: "Quest (campaign B)", DefaultAnnounced: AnnouncedOnDay}); err != nil {
			t.Fatalf("Create in campaign B with the same slug should succeed: %v", err)
		}

		list, err := repo.List(ctx, fixA.CampaignID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("List returned %d kinds, want 2", len(list))
		}
		if list[0].ID != k2.ID || list[1].ID != k1.ID {
			t.Errorf("List order = [%d, %d], want sort_order ascending [%d, %d]", list[0].ID, list[1].ID, k2.ID, k1.ID)
		}
		for _, k := range list {
			if k.CampaignID != fixA.CampaignID {
				t.Fatalf("List(A) returned a kind from campaign %q", k.CampaignID)
			}
		}
	})

	t.Run("tenant isolation: campaign B cannot read, update or delete campaign A's kind", func(t *testing.T) {
		k, err := repo.Create(ctx, fixA.CampaignID, EventKindInput{Slug: "travel", Name: "Travel", DefaultAnnounced: AnnouncedOnDay})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if got, err := repo.GetByID(ctx, k.ID, fixB.CampaignID); err != nil || got != nil {
			t.Errorf("GetByID(k.ID, campaign B) = %+v, %v; want (nil, nil)", got, err)
		}

		err = repo.Update(ctx, k.ID, fixB.CampaignID, EventKindInput{Slug: "travel", Name: "Hijacked", DefaultAnnounced: AnnouncedOnDay})
		if _, ok := err.(*apperror.AppError); !ok || err == nil {
			t.Errorf("Update(k.ID, campaign B) = %v, want a not-found apperror", err)
		}
		stillA, _ := repo.GetByID(ctx, k.ID, fixA.CampaignID)
		if stillA == nil || stillA.Name != "Travel" {
			t.Errorf("campaign B's failed Update leaked through: %+v", stillA)
		}

		if err := repo.Delete(ctx, k.ID, fixB.CampaignID); err == nil {
			t.Error("Delete(k.ID, campaign B) should fail, got nil")
		}
		stillA, _ = repo.GetByID(ctx, k.ID, fixA.CampaignID)
		if stillA == nil {
			t.Error("campaign B's failed Delete removed campaign A's kind")
		}
	})

	t.Run("Create rejects a duplicate slug in the same campaign as a conflict", func(t *testing.T) {
		if _, err := repo.Create(ctx, fixA.CampaignID, EventKindInput{Slug: "dup", Name: "Dup", DefaultAnnounced: AnnouncedOnDay}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		_, err := repo.Create(ctx, fixA.CampaignID, EventKindInput{Slug: "dup", Name: "Dup 2", DefaultAnnounced: AnnouncedOnDay})
		if apperror.SafeCode(err) != http.StatusConflict {
			t.Errorf("Create(duplicate slug) err = %v, want a conflict error", err)
		}
	})

	t.Run("Create and Update reject an unrecognized default_announced value", func(t *testing.T) {
		if _, err := repo.Create(ctx, fixA.CampaignID, EventKindInput{Slug: "bad-ann", Name: "Bad", DefaultAnnounced: "sometimes"}); apperror.SafeCode(err) != http.StatusUnprocessableEntity {
			t.Errorf("Create(DefaultAnnounced=\"sometimes\") err = %v, want a validation error", err)
		}

		k, err := repo.Create(ctx, fixA.CampaignID, EventKindInput{Slug: "good-ann", Name: "Good", DefaultAnnounced: AnnouncedOnDay})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := repo.Update(ctx, k.ID, fixA.CampaignID, EventKindInput{Slug: "good-ann", Name: "Good", DefaultAnnounced: "whenever"}); apperror.SafeCode(err) != http.StatusUnprocessableEntity {
			t.Errorf("Update(DefaultAnnounced=\"whenever\") err = %v, want a validation error", err)
		}
	})

	t.Run("Update with no actual change still succeeds (not a false not-found)", func(t *testing.T) {
		k, err := repo.Create(ctx, fixA.CampaignID, EventKindInput{Slug: "steady", Name: "Steady", Color: "#111111", DefaultAnnounced: AnnouncedOnDay})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		// Save back the exact same values: a production DSN (no
		// clientFoundRows) reports zero rows changed here even though the
		// kind exists.
		if err := repo.Update(ctx, k.ID, fixA.CampaignID, EventKindInput{Slug: "steady", Name: "Steady", Color: "#111111", DefaultAnnounced: AnnouncedOnDay}); err != nil {
			t.Errorf("Update with identical values = %v, want nil (existing row, no real change)", err)
		}
	})

	t.Run("deleting the campaign cascades its kinds", func(t *testing.T) {
		fix := newTestCampaign(t, db, "kind-cascade")
		k, err := repo.Create(ctx, fix.CampaignID, EventKindInput{Slug: "birthday", Name: "Birthday", DefaultAnnounced: AnnouncedOnDay})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, fix.CampaignID)

		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM calendar_event_kinds WHERE id = ?`, k.ID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Error("deleting the campaign should cascade calendar_event_kinds, but the row survived")
		}
	})
}
