// event_batch_integration_test.go checks against a real MariaDB that a batch
// lands whole or not at all: a write that fails late rolls back the events
// and weather written before it. Skipped under `-short`.
package calendar

import (
	"context"
	"testing"
)

func TestEventBatchRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	calRepo, eventRepo, weatherRepo := NewCalendarRepository(db), NewEventRepository(db), NewWeatherRepository(db)
	batch := NewEventBatchRepository(db)
	fix := newTestCampaign(t, db, "batch")
	cal := newTestCalendar(testUUID(t), fix.CampaignID, "Batch")
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("Create calendar: %v", err)
	}
	old := &Event{ID: testUUID(t), CalendarID: cal.ID, Name: "Old", Year: 5, Month: 1, Day: 3, Visibility: "everyone", AllDay: true}
	if err := eventRepo.CreateEvent(ctx, old); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	fresh := func(name string) *Event {
		return &Event{ID: testUUID(t), CalendarID: cal.ID, Name: name, Year: 5, Month: 1, Day: 9, Visibility: "dm_only", AllDay: true}
	}
	day := DayWeatherInput{Year: 5, Month: 1, Day: 9, Source: WeatherSourceGenerated}

	// The update names an event that does not exist, so it fails after the
	// create and the weather would have been written.
	ghost := fresh("Ghost")
	lost := fresh("Lost")
	err := batch.ApplyEventBatch(ctx, cal.ID, EventBatchWrites{
		Create:     []*Event{lost},
		Update:     []*Event{ghost},
		DeleteIDs:  []string{old.ID},
		SetWeather: []DayWeatherInput{day},
	})
	if err == nil {
		t.Fatal("a batch with an update for a missing event was accepted")
	}
	if got, _ := eventRepo.GetEvent(ctx, lost.ID); got != nil {
		t.Error("the created event survived a failed batch")
	}
	if got, _ := eventRepo.GetEvent(ctx, old.ID); got == nil {
		t.Error("the deleted event was removed by a failed batch")
	}
	if days, _ := weatherRepo.ListDays(ctx, cal.ID, 5, 1); len(days) != 0 {
		t.Errorf("weather survived a failed batch: %+v", days)
	}

	// The same batch without the bad item lands whole.
	kept := fresh("Kept")
	if err := batch.ApplyEventBatch(ctx, cal.ID, EventBatchWrites{
		Create:     []*Event{kept},
		DeleteIDs:  []string{old.ID},
		SetWeather: []DayWeatherInput{day},
	}); err != nil {
		t.Fatalf("ApplyEventBatch: %v", err)
	}
	if got, _ := eventRepo.GetEvent(ctx, kept.ID); got == nil {
		t.Error("the created event is missing")
	}
	if got, _ := eventRepo.GetEvent(ctx, old.ID); got != nil {
		t.Error("the deleted event is still there")
	}
	if days, _ := weatherRepo.ListDays(ctx, cal.ID, 5, 1); len(days) != 1 {
		t.Errorf("weather days = %d, want 1", len(days))
	}
}
