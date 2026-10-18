package sessions

import (
	"context"
	"testing"
)

// TestDB_Sightings proves the sightings SQL against real rows: answering the
// same night twice replaces the answer, nights of one series stay separate,
// links come back with their session's date, and soft-deleted sessions and
// other campaigns are left out.
func TestDB_Sightings(t *testing.T) {
	db := newScratchDB(t)
	repo := NewSightingRepository(db)
	ctx := context.Background()

	campID, userID := seedCampaign(t, db)
	otherCamp, otherUser := seedCampaign(t, db)
	res, err := db.Exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?, 'npc', 'NPC', 'NPCs')`, campID)
	if err != nil {
		t.Fatalf("seed entity type: %v", err)
	}
	typeID, _ := res.LastInsertId()
	entityID := newDBID(t)
	if _, err := db.Exec(`INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, visibility, created_by, created_at, updated_at)
		VALUES (?, ?, ?, 'Maudry', 'maudry', false, 'default', ?, NOW(), NOW())`, entityID, campID, typeID, userID); err != nil {
		t.Fatalf("seed entity: %v", err)
	}

	weekly := seedSession(t, db, campID, userID, "Weekly")
	gone := seedSession(t, db, campID, userID, "Deleted")
	elsewhere := seedSession(t, db, otherCamp, otherUser, "Other table")
	if _, err := db.Exec(`UPDATE sessions SET scheduled_date = '2026-10-03', is_recurring = 1, recurrence_type = 'weekly' WHERE id = ?`, weekly); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sessions SET scheduled_date = '2026-09-01', deleted_at = NOW() WHERE id = ?`, gone); err != nil {
		t.Fatal(err)
	}

	write := func(session, date, state string) {
		t.Helper()
		if err := repo.UpsertSighting(ctx, SightingRow{SessionID: session, OccurrenceDate: date, EntityID: entityID, State: state, Source: SightingSourceCheckin, AddedBy: userID}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	write(weekly, "2026-10-10", SightingNotSeen)
	write(weekly, "2026-10-10", SightingSeen) // same night: replaces
	write(weekly, "2026-10-17", SightingNotSeen)
	write(gone, "2026-09-01", SightingSeen)
	write(elsewhere, "2026-09-01", SightingSeen)

	rows, err := repo.ListSightingsForEntity(ctx, campID, entityID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.SessionID+"|"+r.OccurrenceDate] = r.State
		if r.SessionName != "Weekly" || r.AddedBy != userID {
			t.Errorf("row %+v: want session name Weekly and added_by set", r)
		}
	}
	want := map[string]string{weekly + "|2026-10-10": SightingSeen, weekly + "|2026-10-17": SightingNotSeen}
	if len(got) != len(want) || got[weekly+"|2026-10-10"] != SightingSeen || got[weekly+"|2026-10-17"] != SightingNotSeen {
		t.Errorf("rows = %v, want %v", got, want)
	}

	night, err := repo.ListSightingsForNight(ctx, weekly, "2026-10-10")
	if err != nil || len(night) != 1 || night[entityID] != SightingSeen {
		t.Errorf("night 2026-10-10 = %v, %v; want the one seen answer", night, err)
	}

	if err := repo.DeleteSighting(ctx, weekly, "2026-10-17", entityID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if rows, _ = repo.ListSightingsForEntity(ctx, campID, entityID); len(rows) != 1 {
		t.Errorf("after delete: %d rows, want 1", len(rows))
	}

	for _, l := range []struct{ session, role string }{{weekly, EntityRoleEncountered}, {gone, EntityRoleKey}} {
		if _, err := db.Exec(`INSERT INTO session_entities (session_id, entity_id, role) VALUES (?, ?, ?)`, l.session, entityID, l.role); err != nil {
			t.Fatalf("seed link: %v", err)
		}
	}
	links, err := repo.ListLinksForEntity(ctx, campID, entityID)
	if err != nil {
		t.Fatalf("links: %v", err)
	}
	if len(links) != 1 || links[0].SessionID != weekly || links[0].ScheduledDate != "2026-10-03" || !links[0].IsRecurring || links[0].Role != EntityRoleEncountered {
		t.Errorf("links = %+v, want only the weekly series, dated 2026-10-03", links)
	}
}
