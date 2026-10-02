package calendar

import (
	"context"
	"testing"
)

// TestEventRepository_GetEventsByIDs_Integration pins the batch read against
// a real database: calendar scoping in SQL, unknown ids absent, the joined
// kind display fields present, and a list larger than one IN chunk.
func TestEventRepository_GetEventsByIDs_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	eventRepo := NewEventRepository(db)
	calRepo := NewCalendarRepository(db)
	kindRepo := NewEventKindRepository(db)

	fix := newTestCampaign(t, db, "evbyids")
	calA := newTestCalendar(testUUID(t), fix.CampaignID, "Calendar A")
	calB := newTestCalendar(testUUID(t), fix.CampaignID, "Calendar B")
	for _, c := range []*Calendar{calA, calB} {
		if err := calRepo.Create(ctx, c); err != nil {
			t.Fatalf("Create calendar: %v", err)
		}
	}
	kind, err := kindRepo.Create(ctx, fix.CampaignID, EventKindInput{Slug: "festival", Name: "Festival", Icon: "x", Color: "#10b981", DefaultAnnounced: AnnouncedAhead})
	if err != nil {
		t.Fatalf("Create kind: %v", err)
	}

	mk := func(cal *Calendar, name string, withKind bool) string {
		e := &Event{ID: testUUID(t), CalendarID: cal.ID, Name: name, Year: 1, Month: 1, Day: 1, Visibility: "everyone", AllDay: true}
		if withKind {
			e.KindID = &kind.ID
		}
		if err := eventRepo.CreateEvent(ctx, e); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		return e.ID
	}
	a1 := mk(calA, "a1", true)
	a2 := mk(calA, "a2", false)
	b1 := mk(calB, "b1", false)

	got, err := eventRepo.GetEventsByIDs(ctx, calA.ID, []string{a1, a2, b1, "no-such-id"})
	if err != nil {
		t.Fatalf("GetEventsByIDs: %v", err)
	}
	byID := map[string]Event{}
	for _, e := range got {
		byID[e.ID] = e
	}
	if len(got) != 2 || byID[a1].Name != "a1" || byID[a2].Name != "a2" {
		t.Fatalf("got %d events %+v, want exactly a1 and a2 (other-calendar and unknown ids absent)", len(got), got)
	}
	if byID[a1].KindSlug != "festival" {
		t.Errorf("KindSlug = %q, want the joined kind", byID[a1].KindSlug)
	}

	if got, err := eventRepo.GetEventsByIDs(ctx, calA.ID, nil); err != nil || len(got) != 0 {
		t.Errorf("empty ids = %v, %v; want no rows, no error", got, err)
	}

	// More ids than one chunk: the tail beyond eventIDsChunk must still resolve.
	ids := make([]string, 0, eventIDsChunk+2)
	for i := 0; i < eventIDsChunk+1; i++ {
		ids = append(ids, "filler-"+testUUID(t))
	}
	ids = append(ids, a2)
	got, err = eventRepo.GetEventsByIDs(ctx, calA.ID, ids)
	if err != nil || len(got) != 1 || got[0].ID != a2 {
		t.Errorf("chunked read = %+v, %v; want only a2", got, err)
	}
}
